package lua

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

type Driver struct {
	runtime      *Runtime
	managedStore *ManagedScriptStore
}

type DriverConfig struct {
	Script    string         `json:"script"`
	TimeoutMS int            `json:"timeout_ms"`
	Raw       map[string]any `json:"-"`
}

const defaultTimeoutMS = 15000

type DriverOption func(*Driver)

func WithManagedScriptRoot(root string) DriverOption {
	return func(driver *Driver) {
		if driver == nil || strings.TrimSpace(root) == "" {
			return
		}
		store, err := NewManagedScriptStore(root)
		if err == nil {
			driver.managedStore = store
		}
	}
}

func WithManagedScriptStore(store *ManagedScriptStore) DriverOption {
	return func(driver *Driver) {
		if driver != nil {
			driver.managedStore = store
		}
	}
}

func NewDriver(client *http.Client, baseDir string, opts ...DriverOption) *Driver {
	driver := &Driver{
		runtime: NewRuntime(client, baseDir),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(driver)
		}
	}
	return driver
}

func NewDriverWithRuntime(runtime *Runtime) *Driver {
	return &Driver{runtime: runtime}
}

func (d *Driver) Name() string {
	return "lua"
}

func (d *Driver) Supports(account accounts.Account) bool {
	return strings.TrimSpace(account.UsageDriver) == "lua"
}

func (d *Driver) Fetch(ctx context.Context, account accounts.Account, credential accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
	config, err := ParseDriverConfig(account.UsageConfigJSON)
	if err != nil {
		return usagedrv.RawUsageResult{}, err
	}

	timeout := config.TimeoutMS
	if timeout <= 0 {
		timeout = defaultTimeoutMS
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	if d.runtime == nil {
		d.runtime = NewRuntime(nil, "")
	}

	if key, ok := ParseManagedScriptKey(config.Script); ok {
		if d.managedStore == nil {
			return usagedrv.RawUsageResult{}, fmt.Errorf("managed script store is not configured")
		}
		source, err := d.managedStore.Load(key)
		if err != nil {
			if builtInSource, ok := builtInManagedScript(key); ok {
				source = builtInSource
			} else {
				return usagedrv.RawUsageResult{}, err
			}
		}
		return d.executeWithRateLimitRetry(callCtx, source, "managed:"+key, account, credential, config.Raw)
	}

	return d.executeWithRateLimitRetry(callCtx, config.Script, "", account, credential, config.Raw)
}

func (d *Driver) executeWithRateLimitRetry(
	ctx context.Context,
	source string,
	sourceName string,
	account accounts.Account,
	credential accountdrv.ResolvedCredential,
	config map[string]any,
) (usagedrv.RawUsageResult, error) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var (
			result usagedrv.RawUsageResult
			err    error
		)
		if sourceName == "" {
			result, err = d.runtime.Execute(ctx, source, account, credential, config)
		} else {
			result, err = d.runtime.ExecuteSource(ctx, source, sourceName, account, credential, config)
		}
		if err == nil || !isLuaRateLimitError(err) || attempt+1 >= maxAttempts {
			return result, err
		}

		delay := time.Duration(250*(1<<attempt)) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return usagedrv.RawUsageResult{}, ctx.Err()
		case <-timer.C:
		}
	}
	return usagedrv.RawUsageResult{}, fmt.Errorf("lua usage retry exhausted")
}

func isLuaRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	var failure *ScriptFailure
	if errors.As(err, &failure) {
		message := strings.ToLower(failure.Message)
		return strings.Contains(message, "429") || strings.Contains(message, "too many requests")
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "status 429") || strings.Contains(message, "too many requests")
}

func ParseDriverConfig(raw string) (DriverConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DriverConfig{}, fmt.Errorf("lua usage config is empty")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return DriverConfig{}, fmt.Errorf("decode lua usage config: %w", err)
	}

	scriptValue, ok := decoded["script"]
	if !ok {
		return DriverConfig{}, fmt.Errorf("lua usage config missing script")
	}
	script, ok := scriptValue.(string)
	if !ok || strings.TrimSpace(script) == "" {
		return DriverConfig{}, fmt.Errorf("lua usage config script must be non-empty string")
	}

	cfg := DriverConfig{
		Script: strings.TrimSpace(filepath.Clean(script)),
		Raw:    decoded,
	}
	if timeoutValue, ok := decoded["timeout_ms"]; ok {
		timeoutMS, err := parseTimeoutMSStrict(timeoutValue)
		if err != nil {
			return DriverConfig{}, err
		}
		cfg.TimeoutMS = timeoutMS
	} else {
		cfg.TimeoutMS = defaultTimeoutMS
	}
	return cfg, nil
}

func parseTimeoutMSStrict(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("lua usage config timeout_ms must be positive integer")
		}
		timeout := int(typed)
		if timeout <= 0 {
			return 0, fmt.Errorf("lua usage config timeout_ms must be positive integer")
		}
		return timeout, nil
	case int:
		if typed <= 0 {
			return 0, fmt.Errorf("lua usage config timeout_ms must be positive integer")
		}
		return typed, nil
	case int64:
		timeout := int(typed)
		if timeout <= 0 {
			return 0, fmt.Errorf("lua usage config timeout_ms must be positive integer")
		}
		return timeout, nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("lua usage config timeout_ms must be positive integer")
		}
		return parsed, nil
	case nil:
		return 5000, nil
	default:
		return 0, fmt.Errorf("lua usage config timeout_ms has invalid type %T", value)
	}
}

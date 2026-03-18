package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	golua "github.com/yuin/gopher-lua"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

type Runtime struct {
	client  *http.Client
	baseDir string
}

func NewRuntime(client *http.Client, baseDir string) *Runtime {
	return &Runtime{
		client:  client,
		baseDir: baseDir,
	}
}

func (r *Runtime) Execute(ctx context.Context, script string, account accounts.Account, credential accountdrv.ResolvedCredential, config map[string]any) (usagedrv.RawUsageResult, error) {
	resolvedScript, err := r.resolveScriptPath(script)
	if err != nil {
		return usagedrv.RawUsageResult{}, err
	}
	rawScript, err := os.ReadFile(resolvedScript)
	if err != nil {
		return usagedrv.RawUsageResult{}, fmt.Errorf("lua read script: %w", err)
	}
	return r.ExecuteSource(ctx, string(rawScript), resolvedScript, account, credential, config)
}

func (r *Runtime) ExecuteSource(ctx context.Context, source string, sourceName string, account accounts.Account, credential accountdrv.ResolvedCredential, config map[string]any) (usagedrv.RawUsageResult, error) {
	L := golua.NewState(golua.Options{
		SkipOpenLibs: true,
	})
	defer L.Close()
	L.SetContext(ctx)

	if err := r.registerHostAPI(L, ctx); err != nil {
		return usagedrv.RawUsageResult{}, err
	}

	if strings.TrimSpace(sourceName) == "" {
		sourceName = "inline.lua"
	}
	if err := L.DoString(source); err != nil {
		if ctx.Err() != nil {
			return usagedrv.RawUsageResult{}, fmt.Errorf("lua runtime timeout: %w", ctx.Err())
		}
		return usagedrv.RawUsageResult{}, fmt.Errorf("lua load script %s: %w", sourceName, err)
	}

	fn := L.GetGlobal("fetch_usage")
	if fn.Type() != golua.LTFunction {
		return usagedrv.RawUsageResult{}, fmt.Errorf("lua script missing fetch_usage(ctx)")
	}

	ctxValue, err := buildLuaContext(L, account, credential, config)
	if err != nil {
		return usagedrv.RawUsageResult{}, err
	}

	if err := L.CallByParam(golua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, ctxValue); err != nil {
		if ctx.Err() != nil {
			return usagedrv.RawUsageResult{}, fmt.Errorf("lua runtime timeout: %w", ctx.Err())
		}
		return usagedrv.RawUsageResult{}, fmt.Errorf("lua call fetch_usage: %w", err)
	}
	result := L.Get(-1)
	L.Pop(1)
	return decodeScriptResult(result)
}

func (r *Runtime) registerHostAPI(L *golua.LState, ctx context.Context) error {
	host := L.NewTable()
	host.RawSetString("http_get", L.NewFunction(func(state *golua.LState) int {
		arg := state.CheckTable(1)
		urlValue := arg.RawGetString("url")
		urlString, ok := urlValue.(golua.LString)
		if !ok || strings.TrimSpace(string(urlString)) == "" {
			state.RaiseError("http_get requires non-empty url")
			return 0
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, string(urlString), nil)
		if err != nil {
			state.RaiseError("http_get build request: %v", err)
			return 0
		}
		headersValue := arg.RawGetString("headers")
		if headersValue != golua.LNil {
			headersTable, ok := headersValue.(*golua.LTable)
			if !ok {
				state.RaiseError("http_get headers must be table")
				return 0
			}
			headersTable.ForEach(func(key golua.LValue, value golua.LValue) {
				keyString, keyOK := key.(golua.LString)
				valueString, valueOK := value.(golua.LString)
				if !keyOK || !valueOK {
					return
				}
				request.Header.Set(string(keyString), string(valueString))
			})
		}
		client := r.client
		if client == nil {
			client = http.DefaultClient
		}
		response, err := client.Do(request)
		if err != nil {
			state.RaiseError("http_get request failed: %v", err)
			return 0
		}
		defer response.Body.Close()
		body, err := ioReadAll(response.Body)
		if err != nil {
			state.RaiseError("http_get read response: %v", err)
			return 0
		}
		resp := state.NewTable()
		resp.RawSetString("status", golua.LNumber(response.StatusCode))
		resp.RawSetString("body", golua.LString(string(body)))
		respHeaders := state.NewTable()
		for key, values := range response.Header {
			if len(values) == 0 {
				continue
			}
			respHeaders.RawSetString(key, golua.LString(values[0]))
		}
		resp.RawSetString("headers", respHeaders)
		state.Push(resp)
		return 1
	}))
	host.RawSetString("json_decode", L.NewFunction(func(state *golua.LState) int {
		raw := state.CheckString(1)
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			state.RaiseError("json_decode failed: %v", err)
			return 0
		}
		value, err := toLuaValue(state, decoded)
		if err != nil {
			state.RaiseError("json_decode convert: %v", err)
			return 0
		}
		state.Push(value)
		return 1
	}))
	host.RawSetString("json_encode", L.NewFunction(func(state *golua.LState) int {
		value, err := decodeValue(state.CheckAny(1), newDecodeState())
		if err != nil {
			state.RaiseError("json_encode convert: %v", err)
			return 0
		}
		raw, err := json.Marshal(value)
		if err != nil {
			state.RaiseError("json_encode marshal: %v", err)
			return 0
		}
		state.Push(golua.LString(string(raw)))
		return 1
	}))
	host.RawSetString("sleep_ms", L.NewFunction(func(state *golua.LState) int {
		delay := int(state.CheckNumber(1))
		if delay < 0 {
			state.RaiseError("sleep_ms requires non-negative number")
			return 0
		}
		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			state.RaiseError("sleep_ms context done: %v", ctx.Err())
			return 0
		case <-timer.C:
			return 0
		}
	}))
	L.SetGlobal("host", host)
	return nil
}

func buildLuaContext(L *golua.LState, account accounts.Account, credential accountdrv.ResolvedCredential, config map[string]any) (golua.LValue, error) {
	accountTable := L.NewTable()
	accountTable.RawSetString("id", golua.LNumber(account.ID))
	accountTable.RawSetString("provider_type", golua.LString(account.ProviderType))
	accountTable.RawSetString("account_name", golua.LString(account.AccountName))
	accountTable.RawSetString("auth_mode", golua.LString(account.AuthMode))
	accountTable.RawSetString("base_url", golua.LString(account.BaseURL))
	accountTable.RawSetString("usage_driver", golua.LString(account.UsageDriver))

	credentialTable := L.NewTable()
	credentialTable.RawSetString("kind", golua.LString(credential.Kind))
	credentialTable.RawSetString("api_key", golua.LString(credential.APIKey))
	credentialTable.RawSetString("access_token", golua.LString(credential.AccessToken))
	credentialTable.RawSetString("refresh_token", golua.LString(credential.RefreshToken))
	sessionValue, err := toLuaValue(L, credential.Session)
	if err != nil {
		return nil, err
	}
	headersValue, err := toLuaValue(L, credential.Headers)
	if err != nil {
		return nil, err
	}
	metadataValue, err := toLuaValue(L, credential.Metadata)
	if err != nil {
		return nil, err
	}
	credentialTable.RawSetString("session", sessionValue)
	credentialTable.RawSetString("headers", headersValue)
	credentialTable.RawSetString("metadata", metadataValue)

	configValue, err := toLuaValue(L, config)
	if err != nil {
		return nil, err
	}
	root := L.NewTable()
	root.RawSetString("account", accountTable)
	root.RawSetString("credential", credentialTable)
	root.RawSetString("config", configValue)
	root.RawSetString("host", L.GetGlobal("host"))
	return root, nil
}

func toLuaValue(L *golua.LState, value any) (golua.LValue, error) {
	switch typed := value.(type) {
	case nil:
		return golua.LNil, nil
	case bool:
		return golua.LBool(typed), nil
	case string:
		return golua.LString(typed), nil
	case float64:
		return golua.LNumber(typed), nil
	case float32:
		return golua.LNumber(typed), nil
	case int:
		return golua.LNumber(typed), nil
	case int64:
		return golua.LNumber(typed), nil
	case int32:
		return golua.LNumber(typed), nil
	case uint64:
		return golua.LNumber(typed), nil
	case uint32:
		return golua.LNumber(typed), nil
	case map[string]string:
		table := L.NewTable()
		for key, inner := range typed {
			table.RawSetString(key, golua.LString(inner))
		}
		return table, nil
	case map[string]any:
		table := L.NewTable()
		for key, inner := range typed {
			converted, err := toLuaValue(L, inner)
			if err != nil {
				return nil, err
			}
			table.RawSetString(key, converted)
		}
		return table, nil
	case []any:
		table := L.NewTable()
		for _, inner := range typed {
			converted, err := toLuaValue(L, inner)
			if err != nil {
				return nil, err
			}
			table.Append(converted)
		}
		return table, nil
	default:
		return nil, fmt.Errorf("unsupported value for lua context: %T", value)
	}
}

func (r *Runtime) resolveScriptPath(script string) (string, error) {
	script = strings.TrimSpace(script)
	if script == "" {
		return "", fmt.Errorf("usage config missing script path")
	}
	baseDir := r.baseDir
	if strings.TrimSpace(baseDir) == "" {
		moduleRoot, err := findModuleRoot()
		if err != nil {
			return "", err
		}
		baseDir = moduleRoot
	}
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve adapter root: %w", err)
	}
	baseDir, err = filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve adapter root symlinks: %w", err)
	}

	var candidate string
	if filepath.IsAbs(script) {
		candidate = filepath.Clean(script)
	} else {
		candidate = filepath.Clean(filepath.Join(baseDir, script))
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve script path: %w", err)
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve script path symlinks: %w", err)
	}
	rel, err := filepath.Rel(baseDir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve script path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resolve script path: %s outside adapter root", script)
	}
	return candidate, nil
}

func findModuleRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve module root: %w", err)
	}
	dir := cwd
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("resolve module root: go.mod not found from %s", cwd)
		}
		dir = parent
	}
}

func ioReadAll(body io.Reader) ([]byte, error) {
	const maxBodySize = 4 << 20
	limited := &io.LimitedReader{R: body, N: maxBodySize}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	return data, nil
}

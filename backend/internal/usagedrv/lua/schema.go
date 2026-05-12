package lua

import (
	"fmt"
	"time"

	golua "github.com/yuin/gopher-lua"

	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

const maxDecodeDepth = 64

type ScriptFailure struct {
	Kind    string
	Message string
}

type decodeState struct {
	visited map[*golua.LTable]struct{}
	depth   int
}

func newDecodeState() *decodeState {
	return &decodeState{
		visited: make(map[*golua.LTable]struct{}),
	}
}

func (e *ScriptFailure) Error() string {
	return fmt.Sprintf("lua script failure kind=%s: %s", e.Kind, e.Message)
}

func decodeScriptResult(value golua.LValue) (usagedrv.RawUsageResult, error) {
	root, ok := value.(*golua.LTable)
	if !ok {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: top-level value must be a table")
	}

	if err := ensureNoUnknownTopKeys(root); err != nil {
		return usagedrv.RawUsageResult{}, err
	}

	okValue := root.RawGetString("ok")
	okBool, ok := okValue.(golua.LBool)
	if !ok {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: field ok must be boolean")
	}

	if bool(okBool) {
		return decodeSuccessResult(root)
	}
	return usagedrv.RawUsageResult{}, decodeFailureResult(root)
}

func ensureNoUnknownTopKeys(root *golua.LTable) error {
	allowed := map[string]struct{}{
		"ok":         {},
		"source":     {},
		"confidence": {},
		"limits":     {},
		"meta":       {},
		"display":    {},
		"payload":    {},
		"error":      {},
	}
	var err error
	root.ForEach(func(key golua.LValue, _ golua.LValue) {
		if err != nil {
			return
		}
		keyString, ok := key.(golua.LString)
		if !ok {
			err = fmt.Errorf("schema: top-level keys must be strings")
			return
		}
		if _, ok := allowed[string(keyString)]; !ok {
			err = fmt.Errorf("schema: unknown top-level key %q", string(keyString))
		}
	})
	return err
}

func decodeSuccessResult(root *golua.LTable) (usagedrv.RawUsageResult, error) {
	limitsValue := root.RawGetString("limits")
	limitsTable, ok := limitsValue.(*golua.LTable)
	if !ok {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: ok=true requires limits table")
	}

	source := "remote"
	if value := root.RawGetString("source"); value != golua.LNil {
		sourceString, ok := value.(golua.LString)
		if !ok {
			return usagedrv.RawUsageResult{}, fmt.Errorf("schema: source must be string")
		}
		source = string(sourceString)
	}
	if source != "remote" && source != "inferred" && source != "mixed" {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: source must be one of remote|inferred|mixed")
	}

	confidence := "medium"
	if value := root.RawGetString("confidence"); value != golua.LNil {
		confidenceString, ok := value.(golua.LString)
		if !ok {
			return usagedrv.RawUsageResult{}, fmt.Errorf("schema: confidence must be string")
		}
		confidence = string(confidenceString)
	}
	if confidence != "high" && confidence != "medium" && confidence != "low" {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: confidence must be one of high|medium|low")
	}

	limits, err := decodeLimits(limitsTable)
	if err != nil {
		return usagedrv.RawUsageResult{}, err
	}

	metaValue := root.RawGetString("meta")
	meta, err := decodeOptionalMap(metaValue)
	if err != nil {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: meta: %w", err)
	}

	displayValue := root.RawGetString("display")
	display, err := decodeOptionalMap(displayValue)
	if err != nil {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: display: %w", err)
	}

	payloadValue := root.RawGetString("payload")
	payload, err := decodeOptionalMap(payloadValue)
	if err != nil {
		return usagedrv.RawUsageResult{}, fmt.Errorf("schema: payload: %w", err)
	}

	return usagedrv.RawUsageResult{
		Source:     source,
		Confidence: confidence,
		Limits:     limits,
		Meta:       meta,
		Display:    display,
		Payload:    payload,
	}, nil
}

func decodeFailureResult(root *golua.LTable) error {
	errorValue := root.RawGetString("error")
	errorTable, ok := errorValue.(*golua.LTable)
	if !ok {
		return fmt.Errorf("schema: ok=false requires error object")
	}
	kindValue := errorTable.RawGetString("kind")
	kindString, ok := kindValue.(golua.LString)
	if !ok || string(kindString) == "" {
		return fmt.Errorf("schema: error.kind must be non-empty string")
	}
	messageValue := errorTable.RawGetString("message")
	messageString, ok := messageValue.(golua.LString)
	if !ok || string(messageString) == "" {
		return fmt.Errorf("schema: error.message must be non-empty string")
	}
	return &ScriptFailure{
		Kind:    string(kindString),
		Message: string(messageString),
	}
}

func decodeLimits(table *golua.LTable) (usagedrv.UsageLimits, error) {
	var limits usagedrv.UsageLimits
	var err error

	if limits.Balance, err = decodeOptionalNumber(table.RawGetString("balance")); err != nil {
		return limits, fmt.Errorf("schema: limits.balance: %w", err)
	}
	if limits.QuotaRemaining, err = decodeOptionalNumber(table.RawGetString("quota_remaining")); err != nil {
		return limits, fmt.Errorf("schema: limits.quota_remaining: %w", err)
	}
	if limits.RPMRemaining, err = decodeOptionalNumber(table.RawGetString("rpm_remaining")); err != nil {
		return limits, fmt.Errorf("schema: limits.rpm_remaining: %w", err)
	}
	if limits.TPMRemaining, err = decodeOptionalNumber(table.RawGetString("tpm_remaining")); err != nil {
		return limits, fmt.Errorf("schema: limits.tpm_remaining: %w", err)
	}
	if limits.DailyRemaining, err = decodeOptionalNumber(table.RawGetString("daily_remaining")); err != nil {
		return limits, fmt.Errorf("schema: limits.daily_remaining: %w", err)
	}
	if limits.MonthlyRemaining, err = decodeOptionalNumber(table.RawGetString("monthly_remaining")); err != nil {
		return limits, fmt.Errorf("schema: limits.monthly_remaining: %w", err)
	}
	if limits.PrimaryUsedPercent, err = decodeOptionalNumber(table.RawGetString("primary_used_percent")); err != nil {
		return limits, fmt.Errorf("schema: limits.primary_used_percent: %w", err)
	}
	if limits.SecondaryUsedPercent, err = decodeOptionalNumber(table.RawGetString("secondary_used_percent")); err != nil {
		return limits, fmt.Errorf("schema: limits.secondary_used_percent: %w", err)
	}
	if limits.PrimaryResetsAt, err = decodeOptionalRFC3339(table.RawGetString("primary_resets_at")); err != nil {
		return limits, fmt.Errorf("schema: limits.primary_resets_at: %w", err)
	}
	if limits.SecondaryResetsAt, err = decodeOptionalRFC3339(table.RawGetString("secondary_resets_at")); err != nil {
		return limits, fmt.Errorf("schema: limits.secondary_resets_at: %w", err)
	}
	return limits, nil
}

func decodeOptionalNumber(value golua.LValue) (*float64, error) {
	if value == golua.LNil {
		return nil, nil
	}
	number, ok := value.(golua.LNumber)
	if !ok {
		return nil, fmt.Errorf("must be number or nil")
	}
	result := float64(number)
	return &result, nil
}

func decodeOptionalRFC3339(value golua.LValue) (*time.Time, error) {
	if value == golua.LNil {
		return nil, nil
	}
	text, ok := value.(golua.LString)
	if !ok {
		return nil, fmt.Errorf("must be RFC3339 string or nil")
	}
	parsed, err := time.Parse(time.RFC3339, string(text))
	if err != nil {
		return nil, fmt.Errorf("invalid RFC3339 value: %w", err)
	}
	utc := parsed.UTC()
	return &utc, nil
}

func decodeOptionalMap(value golua.LValue) (map[string]any, error) {
	if value == golua.LNil {
		return nil, nil
	}
	table, ok := value.(*golua.LTable)
	if !ok {
		return nil, fmt.Errorf("must be object table or nil")
	}
	return decodeTableToMap(table, newDecodeState())
}

func decodeTableToMap(table *golua.LTable, state *decodeState) (map[string]any, error) {
	if err := state.enter(table); err != nil {
		return nil, err
	}
	defer state.leave(table)

	result := map[string]any{}
	var decodeErr error
	table.ForEach(func(key golua.LValue, value golua.LValue) {
		if decodeErr != nil {
			return
		}
		keyString, ok := key.(golua.LString)
		if !ok {
			decodeErr = fmt.Errorf("object key must be string")
			return
		}
		decoded, err := decodeValue(value, state)
		if err != nil {
			decodeErr = err
			return
		}
		result[string(keyString)] = decoded
	})
	if decodeErr != nil {
		return nil, decodeErr
	}
	return result, nil
}

func decodeValue(value golua.LValue, state *decodeState) (any, error) {
	switch typed := value.(type) {
	case golua.LBool:
		return bool(typed), nil
	case golua.LNumber:
		return float64(typed), nil
	case golua.LString:
		return string(typed), nil
	case *golua.LTable:
		return decodeTableLike(typed, state)
	case *golua.LNilType:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported value type %s", value.Type().String())
	}
}

func decodeTableLike(table *golua.LTable, state *decodeState) (any, error) {
	if err := state.enter(table); err != nil {
		return nil, err
	}
	defer state.leave(table)

	maxIndex := 0
	hasNonArrayKey := false
	table.ForEach(func(key golua.LValue, _ golua.LValue) {
		if hasNonArrayKey {
			return
		}
		number, ok := key.(golua.LNumber)
		if !ok {
			hasNonArrayKey = true
			return
		}
		index := int(number)
		if float64(index) != float64(number) || index < 1 {
			hasNonArrayKey = true
			return
		}
		if index > maxIndex {
			maxIndex = index
		}
	})
	if hasNonArrayKey {
		result := map[string]any{}
		var decodeErr error
		table.ForEach(func(key golua.LValue, value golua.LValue) {
			if decodeErr != nil {
				return
			}
			keyString, ok := key.(golua.LString)
			if !ok {
				decodeErr = fmt.Errorf("object key must be string")
				return
			}
			decoded, err := decodeValue(value, state)
			if err != nil {
				decodeErr = err
				return
			}
			result[string(keyString)] = decoded
		})
		if decodeErr != nil {
			return nil, decodeErr
		}
		return result, nil
	}
	items := make([]any, 0, maxIndex)
	for i := 1; i <= maxIndex; i++ {
		value := table.RawGetInt(i)
		decoded, err := decodeValue(value, state)
		if err != nil {
			return nil, err
		}
		items = append(items, decoded)
	}
	return items, nil
}

func (s *decodeState) enter(table *golua.LTable) error {
	if table == nil {
		return nil
	}
	if _, ok := s.visited[table]; ok {
		return fmt.Errorf("cycle detected in table value")
	}
	if s.depth >= maxDecodeDepth {
		return fmt.Errorf("table value exceeds max depth %d", maxDecodeDepth)
	}
	s.visited[table] = struct{}{}
	s.depth++
	return nil
}

func (s *decodeState) leave(table *golua.LTable) {
	if table == nil {
		return
	}
	delete(s.visited, table)
	if s.depth > 0 {
		s.depth--
	}
}

package groups

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FilterOperator defines the supported comparison operators.
type FilterOperator string

const (
	OpEq        FilterOperator = "eq"
	OpNeq       FilterOperator = "neq"
	OpGT        FilterOperator = "gt"
	OpGTE       FilterOperator = "gte"
	OpLT        FilterOperator = "lt"
	OpLTE       FilterOperator = "lte"
	OpContains  FilterOperator = "contains"
	OpStartsWith FilterOperator = "startswith"
	OpIn        FilterOperator = "in"
	OpContainsAny FilterOperator = "contains_any"
	OpIsNull    FilterOperator = "is_null"
	OpNotNull   FilterOperator = "not_null"
	OpRegex     FilterOperator = "regex"
)

// Supported operators list.
var validOperators = map[FilterOperator]struct{}{
	OpEq: {}, OpNeq: {}, OpGT: {}, OpGTE: {}, OpLT: {}, OpLTE: {},
	OpContains: {}, OpStartsWith: {}, OpIn: {}, OpContainsAny: {},
	OpIsNull: {}, OpNotNull: {}, OpRegex: {},
}

// Filter represents a single filter condition in a Smart Group expression.
type Filter struct {
	Field string      `json:"field"`
	Op    FilterOperator `json:"op"`
	Value interface{} `json:"value"`
}

// Validate checks that the filter has a valid operator and non-empty field.
func (f Filter) Validate() error {
	if f.Field == "" {
		return fmt.Errorf("filter: field is required")
	}
	if _, ok := validOperators[f.Op]; !ok {
		return fmt.Errorf("filter: unsupported operator %q for field %q", f.Op, f.Field)
	}
	return nil
}

// Expression represents a complete Smart Group filter expression.
// Supports AND/OR top-level with flat or nested filters.
type Expression struct {
	Condition string    `json:"condition"` // "AND" or "OR"
	Filters   []Filter  `json:"filters"`
	Nested    *Expression `json:"nested,omitempty"` // for nested expressions
}

// Validate checks the expression structure.
func (e Expression) Validate() error {
	cond := strings.ToUpper(strings.TrimSpace(e.Condition))
	if cond != "AND" && cond != "OR" {
		return fmt.Errorf("expression: condition must be AND or OR, got %q", e.Condition)
	}
	if len(e.Filters) == 0 && e.Nested == nil {
		return fmt.Errorf("expression: must have at least one filter or nested expression")
	}
	for i, f := range e.Filters {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("filter[%d]: %w", i, err)
		}
	}
	if e.Nested != nil {
		if err := e.Nested.Validate(); err != nil {
			return fmt.Errorf("nested: %w", err)
		}
	}
	return nil
}

// Device represents the subset of device properties used for Smart Group evaluation.
// Mirrors the columns available from the devices table.
type Device struct {
	Hostname             string     `json:"hostname"`
	OS                   string     `json:"os"`
	OSVersion            string     `json:"os_version"`
	Arch                 string     `json:"arch"`
	CPUCores             int        `json:"cpu_cores"`
	RAMTotalMB           int64      `json:"ram_total_mb"`
	DiskTotalMB          int64      `json:"disk_total_mb"`
	AgentVersion         string     `json:"agent_version"`
	Status               string     `json:"status"`
	LastHeartbeat        *time.Time `json:"last_heartbeat"`
	CreatedAt            time.Time  `json:"created_at"`
	Tags                 []string   `json:"tags"`
	PublicIP             string     `json:"public_ip"`
	PrivateIPs           []string   `json:"private_ips"`
}

// Evaluate checks whether the given Device matches this expression.
func (e Expression) Evaluate(device Device) (bool, error) {
	if err := e.Validate(); err != nil {
		return false, fmt.Errorf("evaluate: %w", err)
	}

	switch strings.ToUpper(strings.TrimSpace(e.Condition)) {
	case "AND":
		return e.evaluateAnd(device)
	case "OR":
		return e.evaluateOr(device)
	default:
		return false, fmt.Errorf("evaluate: unsupported condition %q", e.Condition)
	}
}

func (e Expression) evaluateAnd(device Device) (bool, error) {
	for _, f := range e.Filters {
		matched, err := evaluateFilter(f, device)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	if e.Nested != nil {
		return e.Nested.evaluateAnd(device)
	}
	return true, nil
}

func (e Expression) evaluateOr(device Device) (bool, error) {
	for _, f := range e.Filters {
		matched, err := evaluateFilter(f, device)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	if e.Nested != nil {
		cond := strings.ToUpper(strings.TrimSpace(e.Nested.Condition))
		if cond == "AND" {
			return e.Nested.evaluateAnd(device)
		}
		return e.Nested.evaluateOr(device)
	}
	return false, nil
}

// evaluateFilter evaluates a single Filter against a Device.
func evaluateFilter(f Filter, device Device) (bool, error) {
	raw := getDeviceValue(f.Field, device)

	switch f.Op {
	case OpIsNull:
		return raw == nil, nil
	case OpNotNull:
		return raw != nil, nil
	}

	if raw == nil {
		return false, nil
	}

	switch f.Op {
	case OpEq:
		return valuesEqual(raw, f.Value), nil
	case OpNeq:
		return !valuesEqual(raw, f.Value), nil
	case OpGT, OpGTE, OpLT, OpLTE:
		return compareNumeric(raw, f.Value, f.Op)
	case OpContains:
		return stringContains(raw, f.Value)
	case OpStartsWith:
		return stringStartsWith(raw, f.Value)
	case OpIn:
		return valueInList(raw, f.Value)
	case OpContainsAny:
		return arrayIntersects(raw, f.Value)
	case OpRegex:
		return matchRegex(raw, f.Value)
	default:
		return false, fmt.Errorf("operator %q not implemented", f.Op)
	}
}

// getDeviceValue extracts the value for a given field name from the device.
func getDeviceValue(field string, device Device) interface{} {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "hostname":
		return device.Hostname
	case "os":
		return device.OS
	case "os_version":
		return device.OSVersion
	case "arch":
		return device.Arch
	case "cpu_cores":
		return device.CPUCores
	case "ram_total_mb":
		return device.RAMTotalMB
	case "disk_total_mb":
		return device.DiskTotalMB
	case "agent_version":
		return device.AgentVersion
	case "status":
		return device.Status
	case "last_heartbeat":
		return device.LastHeartbeat
	case "created_at":
		return device.CreatedAt
	case "public_ip":
		return device.PublicIP
	case "private_ips":
		return device.PrivateIPs
	case "tags":
		return device.Tags
	default:
		return nil
	}
}

// valuesEqual compares two values for equality.
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch va := a.(type) {
	case string:
		if vb, ok := b.(string); ok {
			return strings.EqualFold(va, vb)
		}
		return false
	case int:
		if vb, ok := b.(float64); ok {
			return float64(va) == vb
		}
		if vb, ok := b.(int); ok {
			return va == vb
		}
		return false
	case int64:
		if vb, ok := b.(float64); ok {
			return float64(va) == vb
		}
		if vb, ok := b.(int); ok {
			return float64(va) == float64(vb)
		}
		if vb, ok := b.(int64); ok {
			return va == vb
		}
		return false
	case bool:
		if vb, ok := b.(bool); ok {
			return va == vb
		}
		return false
	case time.Time:
		if vb, ok := b.(time.Time); ok {
			return va.Equal(vb)
		}
		return false
	case *time.Time:
		if vb, ok := b.(*time.Time); ok {
			if va == nil && vb == nil {
				return true
			}
			if va == nil || vb == nil {
				return false
			}
			return va.Equal(*vb)
		}
		return false
	case []string:
		vb, ok := b.([]string)
		if !ok {
			return false
		}
		if len(va) != len(vb) {
			return false
		}
		for i := range va {
			if va[i] != vb[i] {
				return false
			}
		}
		return true
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}

// compareNumeric compares numeric values using the given operator.
// Handles int, int64, float64, string (parsed), and time.Time values.
func compareNumeric(a, b interface{}, op FilterOperator) (bool, error) {
	aNum := toFloat64(a)
	bNum := toFloat64(b)

	// Time values convert to float64 via UnixNano, so 0 comparison check needs adjustment
	if aNum == 0 && !isTime(a) {
		return false, fmt.Errorf("cannot compare %T %v with numeric operator %s", a, a, op)
	}
	if bNum == 0 && !isTime(b) {
		return false, fmt.Errorf("cannot compare %v with numeric operator %s", b, op)
	}

	switch op {
	case OpGT:
		return aNum > bNum, nil
	case OpGTE:
		return aNum >= bNum, nil
	case OpLT:
		return aNum < bNum, nil
	case OpLTE:
		return aNum <= bNum, nil
	}
	return false, nil
}

// toFloat64 converts a value to float64 for numeric comparison.
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0
		}
		return f
	case time.Time:
		return float64(val.UnixNano())
	case *time.Time:
		if val == nil {
			return 0
		}
		return float64(val.UnixNano())
	default:
		return 0
	}
}

// isTime reports whether v is a time.Time value.
func isTime(v interface{}) bool {
	switch v.(type) {
	case time.Time, *time.Time:
		return true
	default:
		return false
	}
}

// stringContains checks if the string value contains the substring (case-insensitive).
func stringContains(a, b interface{}) (bool, error) {
	str := toString(a)
	sub := toString(b)
	return strings.Contains(strings.ToLower(str), strings.ToLower(sub)), nil
}

// stringStartsWith checks if the string starts with the prefix (case-insensitive).
func stringStartsWith(a, b interface{}) (bool, error) {
	str := toString(a)
	prefix := toString(b)
	return strings.HasPrefix(strings.ToLower(str), strings.ToLower(prefix)), nil
}

// valueInList checks if the value is in the given list/array.
func valueInList(v, list interface{}) (bool, error) {
	arr, ok := toAnySlice(list)
	if !ok {
		return false, fmt.Errorf("value_in_list: expected array for value %v", list)
	}
	for _, item := range arr {
		if valuesEqual(v, item) {
			return true, nil
		}
	}
	return false, nil
}

// arrayIntersects checks if the first array has any elements in common with the second array.
func arrayIntersects(a, b interface{}) (bool, error) {
	arrA, ok1 := toAnySlice(a)
	arrB, ok2 := toAnySlice(b)
	if !ok1 || !ok2 {
		return false, fmt.Errorf("contains_any: both values must be arrays")
	}
	for _, itemA := range arrA {
		for _, itemB := range arrB {
			if valuesEqual(itemA, itemB) {
				return true, nil
			}
		}
	}
	return false, nil
}

// matchRegex checks if the string value matches the regex pattern.
func matchRegex(v, pattern interface{}) (bool, error) {
	str := toString(v)
	patternStr := toString(pattern)
	matched, err := regexp.MatchString(patternStr, str)
	if err != nil {
		return false, fmt.Errorf("regex match: %w", err)
	}
	return matched, nil
}

// toString converts a value to string for string operations.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case time.Time:
		return val.Format(time.RFC3339)
	case *time.Time:
		if val == nil {
			return ""
		}
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// toAnySlice attempts to convert a value to []interface{}.
func toAnySlice(v interface{}) ([]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	arr, ok := v.([]interface{})
	if ok {
		return arr, true
	}
	// Also handle []string for JSON-unmarshaled arrays
	strArr, ok := v.([]string)
	if ok {
		result := make([]interface{}, len(strArr))
		for i, s := range strArr {
			result[i] = s
		}
		return result, true
	}
	return nil, false
}

// ParseExpression unmarshals a JSON byte slice into an Expression.
func ParseExpression(data []byte) (Expression, error) {
	var expr Expression
	if err := json.Unmarshal(data, &expr); err != nil {
		return Expression{}, fmt.Errorf("parse expression: %w", err)
	}
	if err := expr.Validate(); err != nil {
		return Expression{}, fmt.Errorf("parse expression: %w", err)
	}
	return expr, nil
}

// SerializeExpression marshals an Expression back to JSON bytes.
func SerializeExpression(expr Expression) ([]byte, error) {
	if err := expr.Validate(); err != nil {
		return nil, fmt.Errorf("serialize: %w", err)
	}
	return json.Marshal(expr)
}

// IsSmartGroup returns whether a device group uses smart (dynamic) filtering.
// Groups with a non-empty filter_expression are considered smart.
func IsSmartGroup(filterExpression json.RawMessage) bool {
	if filterExpression == nil {
		return false
	}
	trimmed := strings.TrimSpace(string(filterExpression))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return false
	}
	// Check if it has actual filter content
	var expr Expression
	if err := json.Unmarshal(filterExpression, &expr); err != nil {
		return false
	}
	return len(expr.Filters) > 0 || expr.Nested != nil
}

package jsutil

import (
	"math"
	"strings"
)

// Option values come from JSON and keep JavaScript's dynamic typing: nil is
// null, and booleans, float64 numbers, strings, []any arrays and *Object[any]
// objects keep their JSON types. Undefined values are
// represented by Undefined so that a missing key and an explicit null differ.

// UndefinedValue is the type of Undefined.
type UndefinedValue struct{}

// Undefined stands for JavaScript's undefined.
var Undefined = UndefinedValue{}

// IsUndefined reports whether v is undefined.
func IsUndefined(v any) bool {
	_, ok := v.(UndefinedValue)
	return ok
}

// Truthy implements JavaScript truthiness.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil, UndefinedValue:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case int:
		return x != 0
	case string:
		return x != ""
	}
	return true
}

// ToNumber implements the ToNumber conversion.
func ToNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case UndefinedValue:
		return math.NaN()
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		return StringToNumber(x)
	case []any:
		return StringToNumber(ToString(x))
	case []string:
		return StringToNumber(strings.Join(x, ","))
	}
	return math.NaN()
}

// ToString implements the ToString conversion (String(v)).
func ToString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case UndefinedValue:
		return "undefined"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return FormatNumber(x)
	case int:
		return FormatNumber(float64(x))
	case string:
		return x
	case []string:
		return strings.Join(x, ",")
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			if item == nil || IsUndefined(item) {
				continue
			}
			parts[i] = ToString(item)
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

func isObject(v any) bool {
	switch v.(type) {
	case []any, []string, *Object[any]:
		return true
	}
	return false
}

// AsObject returns v as an object when it is one.
func AsObject(v any) (*Object[any], bool) {
	o, ok := v.(*Object[any])
	return o, ok && o != nil
}

func toPrimitive(v any) any {
	if isObject(v) {
		return ToString(v)
	}
	return v
}

// LooseEqual implements the == operator for option values.
func LooseEqual(a, b any) bool {
	if isObject(a) && isObject(b) {
		return false
	}
	a, b = toPrimitive(a), toPrimitive(b)
	aNull := a == nil || IsUndefined(a)
	bNull := b == nil || IsUndefined(b)
	if aNull || bNull {
		return aNull && bNull
	}
	as, aIsString := a.(string)
	bs, bIsString := b.(string)
	if aIsString && bIsString {
		return as == bs
	}
	if ab, ok := a.(bool); ok && !isNumberValue(b) && !bIsString {
		bb, ok := b.(bool)
		return ok && ab == bb
	}
	return ToNumber(a) == ToNumber(b)
}

func isNumberValue(v any) bool {
	switch v.(type) {
	case float64, int:
		return true
	}
	return false
}

// StrictEqual implements the === operator for primitive option values.
func StrictEqual(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case UndefinedValue:
		return IsUndefined(b)
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case float64, int:
		return isNumberValue(b) && ToNumber(a) == ToNumber(b)
	}
	return false
}

// LessThan implements a < b for option values.
func LessThan(a, b any) bool {
	a, b = toPrimitive(a), toPrimitive(b)
	as, aIsString := a.(string)
	bs, bIsString := b.(string)
	if aIsString && bIsString {
		return CompareUTF16(as, bs) < 0
	}
	x, y := ToNumber(a), ToNumber(b)
	return x < y
}

// TypeOf implements the typeof operator for option values.
func TypeOf(v any) string {
	switch v.(type) {
	case UndefinedValue:
		return "undefined"
	case bool:
		return "boolean"
	case float64, int:
		return "number"
	case string:
		return "string"
	}
	return "object"
}

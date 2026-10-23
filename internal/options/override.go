// Package options normalizes the user's options (compatibility, format,
// optimization level, inlining and rebasing) the way the original does,
// keeping JavaScript's handling of loosely typed option values.
package options

import "github.com/clean-css/clean-css/internal/jsutil"

// Object is an option object decoded from JSON.
type Object = *jsutil.Object[any]

func newObject() Object {
	return jsutil.NewObject[any]()
}

// objectFrom builds an option object from key and value pairs.
func objectFrom(pairs ...any) Object {
	o := newObject()
	for i := 0; i+1 < len(pairs); i += 2 {
		o.Set(pairs[i].(string), pairs[i+1])
	}
	return o
}

// override returns a copy of source1 with the values of source2 laid over
// it: arrays are copied and nested objects are overridden recursively.
func override(source1, source2 any) Object {
	target := newObject()

	if o, ok := jsutil.AsObject(source1); ok {
		for _, key := range o.Keys() {
			item, _ := o.Get(key)
			target.Set(key, copyValue(item))
		}
	} else if s, ok := source1.(string); ok {
		for i := 0; i < len(s); i++ {
			target.Set(jsutil.FormatNumber(float64(i)), s[i:i+1])
		}
	}

	if o, ok := jsutil.AsObject(source2); ok {
		for _, key := range o.Keys() {
			item, _ := o.Get(key)
			switch v := item.(type) {
			case []any:
				target.Set(key, append([]any{}, v...))
			case *jsutil.Object[any]:
				if target.Has(key) && v != nil {
					target.Set(key, override(target.Lookup(key), v))
				} else {
					target.Set(key, item)
				}
			default:
				target.Set(key, item)
			}
		}
	}

	return target
}

func copyValue(item any) any {
	switch v := item.(type) {
	case []any:
		return append([]any{}, v...)
	case *jsutil.Object[any]:
		if v == nil {
			return item
		}
		return override(v, nil)
	}
	return item
}

// lookup reads key from an option value that may not be an object at all.
func lookup(value any, key string) any {
	if o, ok := jsutil.AsObject(value); ok {
		return o.Lookup(key)
	}
	if s, ok := value.(string); ok {
		if i := jsutil.StringToNumber(key); jsutil.IsInteger(i) && i >= 0 && int(i) < len(s) && jsutil.FormatNumber(i) == key {
			return s[int(i) : int(i)+1]
		}
	}
	return jsutil.Undefined
}

// has implements the in operator on an option value.
func has(value any, key string) bool {
	o, ok := jsutil.AsObject(value)
	return ok && o.Has(key)
}

// Values is a group of options, such as the options of one optimization level.
type Values struct {
	Object
}

// Get returns the value of an option, Undefined when it is not set.
func (v Values) Get(name string) any {
	return v.Lookup(name)
}

// Bool returns the truthiness of an option.
func (v Values) Bool(name string) bool {
	return jsutil.Truthy(v.Lookup(name))
}

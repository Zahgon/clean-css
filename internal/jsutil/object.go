package jsutil

import (
	"sort"
	"strconv"
)

// Object is a string-keyed map that iterates like a JavaScript object: keys
// that are array indices come first in ascending numeric order, then the
// other keys in insertion order. Deleting a key and setting it again moves it
// to the end.
type Object[V any] struct {
	keys   []string
	values map[string]V
}

// NewObject returns an empty Object.
func NewObject[V any]() *Object[V] {
	return &Object[V]{values: map[string]V{}}
}

// Get returns the value stored under key.
func (o *Object[V]) Get(key string) (V, bool) {
	v, ok := o.values[key]
	return v, ok
}

// Has reports whether key is present.
func (o *Object[V]) Has(key string) bool {
	_, ok := o.values[key]
	return ok
}

// Set stores value under key, keeping the position of an existing key.
func (o *Object[V]) Set(key string, value V) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// Delete removes key.
func (o *Object[V]) Delete(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			break
		}
	}
}

// Len returns the number of keys.
func (o *Object[V]) Len() int {
	return len(o.keys)
}

// Keys returns the keys in JavaScript property order.
func (o *Object[V]) Keys() []string {
	var indices, names []string
	for _, k := range o.keys {
		if isArrayIndex(k) {
			indices = append(indices, k)
		} else {
			names = append(names, k)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		a, _ := strconv.ParseUint(indices[i], 10, 64)
		b, _ := strconv.ParseUint(indices[j], 10, 64)
		return a < b
	})
	return append(indices, names...)
}

func isArrayIndex(key string) bool {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(key, 10, 64)
	return err == nil && n < 4294967295
}

// Lookup returns the value stored under key, or Undefined when it is absent.
func (o *Object[V]) Lookup(key string) any {
	if o == nil {
		return Undefined
	}
	v, ok := o.values[key]
	if !ok {
		return Undefined
	}
	return v
}

// Copy returns a shallow copy that keeps the key order.
func (o *Object[V]) Copy() *Object[V] {
	c := &Object[V]{keys: append([]string(nil), o.keys...), values: make(map[string]V, len(o.values))}
	for k, v := range o.values {
		c.values[k] = v
	}
	return c
}

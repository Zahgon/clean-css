package level2

import (
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
)

// splice implements Array.prototype.splice on a slice: deleteCount items
// from start are replaced with items, with JavaScript's clamping of
// out-of-range arguments. The slice is modified in place where possible.
func splice[T any](list []T, start, deleteCount int, items ...T) []T {
	if start < 0 {
		start += len(list)
		if start < 0 {
			start = 0
		}
	}
	if start > len(list) {
		start = len(list)
	}
	if deleteCount < 0 {
		deleteCount = 0
	}
	if start+deleteCount > len(list) {
		deleteCount = len(list) - start
	}
	tail := slices.Clone(list[start+deleteCount:])
	list = append(list[:start], items...)
	return append(list, tail...)
}

// sliceRange implements Array.prototype.slice(begin, end) with non-negative
// arguments clamped to the slice; the result is a copy.
func sliceRange[T any](list []T, begin, end int) []T {
	begin = min(max(begin, 0), len(list))
	end = min(max(end, 0), len(list))
	if end < begin {
		end = begin
	}
	return slices.Clone(list[begin:end])
}

// indexFrom implements String.prototype.indexOf with a start position.
func indexFrom(s, substr string, from int) int {
	from = min(max(from, 0), len(s))
	at := strings.Index(s[from:], substr)
	if at < 0 {
		return -1
	}
	return at + from
}

// lookup returns the descriptor of a property name, nil when the property
// has no configuration.
func lookup(name string) *optimizer.Descriptor {
	d, ok := optimizer.Lookup(name)
	if !ok {
		return nil
	}
	return d
}

// containsToken reports whether list holds this very token.
func containsToken(list []*token.Token, t *token.Token) bool {
	return slices.Contains(list, t)
}

// firstValue returns the text of the first value of a property token.
func firstValue(t *token.Token) (string, bool) {
	if len(t.Values) == 0 {
		return "", false
	}
	return t.Values[0].Value, true
}

// valueText returns the text of value i of a wrapped property.
func valueText(p *optimizer.Property, i int) string {
	if i < 0 || i >= len(p.Value) {
		return ""
	}
	return p.Value[i].Value
}

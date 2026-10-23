// Package strutil holds the string helpers shared by the tokenizer, the
// optimizers and the reader: splitting outside of round brackets and natural
// comparison.
package strutil

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/clean-css/clean-css/internal/jsutil"
)

// Split splits value at separator wherever it is not inside round brackets.
// A separator at the very start or end of value does not split it.
func Split(value, separator string) []string {
	if !strings.Contains(value, separator) {
		return []string{value}
	}
	if !strings.Contains(value, "(") {
		return strings.Split(value, separator)
	}
	return splitAt(value, func(c string) bool { return c == separator }, false)
}

// SplitKeep splits value outside of round brackets at every character
// matching re, keeping each separator as an item of its own.
func SplitKeep(value string, re *regexp.Regexp) []string {
	if !strings.Contains(value, "(") {
		return jsutil.Split(re, value)
	}
	return splitAt(value, re.MatchString, true)
}

func splitAt(value string, isSeparator func(string) bool, keep bool) []string {
	level := 0
	cursor := 0
	lastStart := 0
	var parts []string

	for cursor < len(value) {
		_, size := utf8.DecodeRuneInString(value[cursor:])
		character := value[cursor : cursor+size]
		switch character {
		case "(":
			level++
		case ")":
			level--
		}

		if level == 0 && cursor > 0 && cursor+size < len(value) && isSeparator(character) {
			parts = append(parts, value[lastStart:cursor])
			if keep {
				parts = append(parts, character)
			}
			lastStart = cursor + size
		}

		cursor += size
	}

	if lastStart < cursor+1 {
		lastValue := value[lastStart:]
		if _, size := utf8.DecodeLastRuneInString(lastValue); size > 0 && isSeparator(lastValue[len(lastValue)-size:]) {
			lastValue = lastValue[:len(lastValue)-size]
		}
		parts = append(parts, lastValue)
	}

	return parts
}

var numberPattern = regexp.MustCompile(`([0-9]+)`)

// naturalKey is one chunk of a natural comparison: a number when the chunk
// is the canonical spelling of an integer, otherwise the text itself.
type naturalKey struct {
	text     string
	number   float64
	isNumber bool
}

func naturalKeys(value string) []naturalKey {
	chunks := jsutil.Split(numberPattern, value)
	keys := make([]naturalKey, len(chunks))
	for i, chunk := range chunks {
		parsed := jsutil.ParseInt(chunk)
		if jsutil.FormatNumber(parsed) == chunk {
			keys[i] = naturalKey{number: parsed, isNumber: true}
		} else {
			keys[i] = naturalKey{text: chunk}
		}
	}
	return keys
}

func (k naturalKey) value() any {
	if k.isNumber {
		return k.number
	}
	return k.text
}

// NaturalCompare compares two strings so that runs of digits compare as
// numbers, returning -1, 0 or 1. Like the original it relies on JavaScript's
// mixed number and string comparison for chunks such as "007".
func NaturalCompare(value1, value2 string) int {
	keys1 := naturalKeys(value1)
	keys2 := naturalKeys(value2)
	compareFirst := int(math.Min(float64(len(keys1)), float64(len(keys2))))

	for i := 0; i < compareFirst; i++ {
		key1, key2 := keys1[i].value(), keys2[i].value()
		if !jsutil.LooseEqual(key1, key2) {
			if jsutil.LessThan(key2, key1) {
				return 1
			}
			return -1
		}
	}

	switch {
	case len(keys1) > len(keys2):
		return 1
	case len(keys1) == len(keys2):
		return 0
	}
	return -1
}

package jsutil

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// IsSpace reports whether r is white space or a line terminator for
// String.prototype.trim and the \s regular expression class.
func IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// Trim implements String.prototype.trim.
func Trim(s string) string {
	return strings.TrimFunc(s, IsSpace)
}

// TrimStart implements String.prototype.trimStart.
func TrimStart(s string) string {
	return strings.TrimLeftFunc(s, IsSpace)
}

// TrimEnd implements String.prototype.trimEnd (trimRight).
func TrimEnd(s string) string {
	return strings.TrimRightFunc(s, IsSpace)
}

// Len16 returns the length of s in UTF-16 code units, the length JavaScript reports.
func Len16(s string) int {
	n := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			n++
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		n += utf16.RuneLen(r)
		i += size
	}
	return n
}

// CompareUTF16 compares two strings by their UTF-16 code units, the order of
// JavaScript's relational operators and default sort.
func CompareUTF16(a, b string) int {
	if a == b {
		return 0
	}
	ua := utf16.Encode([]rune(a))
	ub := utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			if ua[i] < ub[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ua) < len(ub):
		return -1
	case len(ua) > len(ub):
		return 1
	}
	return 0
}

// Repeat implements String.prototype.repeat for a count that went through
// JavaScript arithmetic: NaN counts as zero.
func Repeat(s string, count float64) string {
	if count != count || count < 1 {
		return ""
	}
	return strings.Repeat(s, int(count))
}

// ReplaceFirst implements String.prototype.replace with a string pattern:
// only the first occurrence is replaced and the replacement understands the
// $$, $& and $` / $' substitutions.
func ReplaceFirst(s, pattern, replacement string) string {
	at := strings.Index(s, pattern)
	if at < 0 {
		return s
	}
	expanded := expandTemplate(replacement, s, []int{at, at + len(pattern)})
	return s[:at] + expanded + s[at+len(pattern):]
}

// CharAt returns the byte at index i of s as a one character string, or the
// empty string when i is out of range, like s[i] on an ASCII string.
func CharAt(s string, i int) string {
	if i < 0 || i >= len(s) {
		return ""
	}
	return s[i : i+1]
}

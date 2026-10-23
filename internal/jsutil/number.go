// Package jsutil reproduces the parts of JavaScript's value semantics that the
// minifier's output depends on: number parsing and printing, loose
// comparisons of option values, string lengths in UTF-16 code units, regular
// expression replacement, insertion-ordered objects and the engine's sort.
package jsutil

import (
	"math"
	"strconv"
	"strings"
)

// FormatNumber formats f the way Number.prototype.toString does.
func FormatNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case f == 0:
		return "0"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f < 0:
		return "-" + FormatNumber(-f)
	}

	repr := strconv.FormatFloat(f, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(repr, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	exp, _ := strconv.Atoi(exponent)
	k := len(digits)
	n := exp + 1

	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}

	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	magnitude := strconv.Itoa(abs(n - 1))
	if k == 1 {
		return digits + "e" + sign + magnitude
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + magnitude
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ParseInt implements the global parseInt with no radix argument: leading
// white space is skipped, an optional sign and a 0x prefix are honoured and
// parsing stops at the first character that is not a digit. NaN is returned
// when no digit is found.
func ParseInt(s string) float64 {
	s = TrimStart(s)
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}

	radix := 10
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		radix = 16
		s = s[2:]
	}

	end := 0
	for end < len(s) && digitValue(s[end]) < radix {
		end++
	}
	if end == 0 {
		return math.NaN()
	}

	var value float64
	if radix == 10 {
		value, _ = strconv.ParseFloat(s[:end], 64)
	} else {
		for i := 0; i < end; i++ {
			value = value*float64(radix) + float64(digitValue(s[i]))
		}
	}
	if negative {
		return -value
	}
	return value
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 36
}

// ParseFloat implements the global parseFloat: the longest prefix of s (after
// leading white space) that is a decimal literal or Infinity is converted.
func ParseFloat(s string) float64 {
	s = TrimStart(s)
	sign := ""
	rest := s
	if rest != "" && (rest[0] == '+' || rest[0] == '-') {
		sign = rest[:1]
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "Infinity") {
		if sign == "-" {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}

	end := scanDecimal(rest)
	if end == 0 {
		return math.NaN()
	}
	value, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil && !math.IsInf(value, 0) {
		return math.NaN()
	}
	if sign == "-" {
		return -value
	}
	return value
}

// scanDecimal returns the length of the longest prefix of s that is an
// unsigned decimal literal (digits, an optional fraction and exponent).
func scanDecimal(s string) int {
	i := 0
	intDigits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		intDigits++
	}
	fracDigits := 0
	if i < len(s) && s[i] == '.' {
		j := i + 1
		for j < len(s) && isDigit(s[j]) {
			j++
			fracDigits++
		}
		if intDigits > 0 || fracDigits > 0 {
			i = j
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return 0
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		k := j
		for k < len(s) && isDigit(s[k]) {
			k++
		}
		if k > j {
			i = k
		}
	}
	return i
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// StringToNumber converts a string the way the unary plus operator does: the
// whole string, trimmed, has to be a numeric literal or it is NaN.
func StringToNumber(s string) float64 {
	s = Trim(s)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		radix := 0
		switch s[1] {
		case 'x', 'X':
			radix = 16
		case 'o', 'O':
			radix = 8
		case 'b', 'B':
			radix = 2
		}
		if radix > 0 {
			var value float64
			for i := 2; i < len(s); i++ {
				d := digitValue(s[i])
				if d >= radix {
					return math.NaN()
				}
				value = value*float64(radix) + float64(d)
			}
			return value
		}
	}

	sign := 1.0
	rest := s
	if rest[0] == '+' || rest[0] == '-' {
		if rest[0] == '-' {
			sign = -1
		}
		rest = rest[1:]
	}
	if rest == "Infinity" {
		return sign * math.Inf(1)
	}
	if end := scanDecimal(rest); end == 0 || end != len(rest) {
		return math.NaN()
	}
	value, err := strconv.ParseFloat(rest, 64)
	if err != nil && !math.IsInf(value, 0) {
		return math.NaN()
	}
	return sign * value
}

// Round implements Math.round: halves round towards positive infinity.
func Round(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return f
	}
	// The explicit conversion rounds f to float64 first: without it the
	// compiler may fuse a caller's multiplication into this subtraction.
	f = float64(f)
	floor := math.Floor(f)
	if f-floor >= 0.5 {
		return floor + 1
	}
	return floor
}

// ToInt32 implements the ToInt32 conversion used by bitwise operators such as ~~x.
func ToInt32(f float64) int32 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	t := math.Trunc(f)
	m := math.Mod(t, 4294967296)
	if m < 0 {
		m += 4294967296
	}
	return int32(uint32(m))
}

// IsInteger reports whether f is a finite number without a fraction.
func IsInteger(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f)
}

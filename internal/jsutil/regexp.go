package jsutil

import (
	"regexp"
	"strconv"
	"strings"
)

// jsSpaceClass lists the characters JavaScript's \s matches, written for use
// inside a Go character class.
const jsSpaceClass = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// jsDot is what an unescaped . matches in a JavaScript pattern without the s flag.
const jsDot = `[^\n\r\x{2028}\x{2029}]`

// MustCompile compiles a JavaScript regular expression source with the given
// flags ("i" is honoured; "g" only matters to the caller) into an equivalent
// Go expression. \s, \S and . are rewritten to JavaScript's character sets.
// Look-around assertions and back references are not supported.
func MustCompile(source, flags string) *regexp.Regexp {
	var b strings.Builder
	if strings.Contains(flags, "i") {
		b.WriteString("(?i)")
	}
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		switch {
		case c == '\\' && i+1 < len(source):
			next := source[i+1]
			i++
			switch {
			case next == 's' && inClass:
				b.WriteString(jsSpaceClass)
			case next == 's':
				b.WriteString("[" + jsSpaceClass + "]")
			case next == 'S' && !inClass:
				b.WriteString("[^" + jsSpaceClass + "]")
			case next == 'S':
				panic("jsutil: \\S inside a character class is not supported: " + source)
			case next >= '1' && next <= '9':
				panic("jsutil: back references are not supported: " + source)
			case next == '/':
				b.WriteByte('/')
			default:
				b.WriteByte('\\')
				b.WriteByte(next)
			}
		case c == '[' && !inClass:
			inClass = true
			b.WriteByte(c)
			if i+1 < len(source) && source[i+1] == '^' {
				b.WriteByte('^')
				i++
			}
			if i+1 < len(source) && source[i+1] == ']' {
				b.WriteString(`\]`)
				i++
			}
		case c == ']' && inClass:
			inClass = false
			b.WriteByte(c)
		case c == '.' && !inClass:
			b.WriteString(jsDot)
		case c == '(' && !inClass && (strings.HasPrefix(source[i:], "(?=") || strings.HasPrefix(source[i:], "(?!")):
			panic("jsutil: look-ahead is not supported: " + source)
		default:
			b.WriteByte(c)
		}
	}
	return regexp.MustCompile(b.String())
}

// Match is one match of a regular expression: the matched text, its byte
// offset and the capture groups (Groups[0] is the whole match). A group that
// did not participate is reported with Matched[i] false.
type Match struct {
	Input   string
	Index   int
	Groups  []string
	Matched []bool
}

// Text returns the whole match.
func (m Match) Text() string {
	return m.Groups[0]
}

func newMatch(input string, loc []int) Match {
	n := len(loc) / 2
	m := Match{Input: input, Index: loc[0], Groups: make([]string, n), Matched: make([]bool, n)}
	for i := 0; i < n; i++ {
		if loc[2*i] >= 0 {
			m.Groups[i] = input[loc[2*i]:loc[2*i+1]]
			m.Matched[i] = true
		}
	}
	return m
}

// Exec returns the first match of re in s, like RegExp.prototype.exec on a
// non-global expression.
func Exec(re *regexp.Regexp, s string) (Match, bool) {
	loc := re.FindStringSubmatchIndex(s)
	if loc == nil {
		return Match{}, false
	}
	return newMatch(s, loc), true
}

// Replace implements String.prototype.replace with a regular expression and a
// replacement pattern. When global is false only the first match is replaced.
// The pattern understands $$, $&, $`, $' and $n / $nn group references.
func Replace(re *regexp.Regexp, s, replacement string, global bool) string {
	return replace(re, s, global, func(loc []int) string {
		return expandTemplate(replacement, s, loc)
	})
}

// ReplaceFunc implements String.prototype.replace with a regular expression
// and a replacer function.
func ReplaceFunc(re *regexp.Regexp, s string, global bool, fn func(Match) string) string {
	return replace(re, s, global, func(loc []int) string {
		return fn(newMatch(s, loc))
	})
}

func replace(re *regexp.Regexp, s string, global bool, fn func([]int) string) string {
	var locs [][]int
	if global {
		locs = re.FindAllStringSubmatchIndex(s, -1)
	} else if loc := re.FindStringSubmatchIndex(s); loc != nil {
		locs = [][]int{loc}
	}
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		b.WriteString(s[last:loc[0]])
		b.WriteString(fn(loc))
		last = loc[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func expandTemplate(tmpl, input string, loc []int) string {
	if !strings.Contains(tmpl, "$") {
		return tmpl
	}
	groups := len(loc)/2 - 1
	group := func(n int) string {
		if loc[2*n] < 0 {
			return ""
		}
		return input[loc[2*n]:loc[2*n+1]]
	}
	var b strings.Builder
	for i := 0; i < len(tmpl); i++ {
		c := tmpl[i]
		if c != '$' || i+1 >= len(tmpl) {
			b.WriteByte(c)
			continue
		}
		next := tmpl[i+1]
		switch {
		case next == '$':
			b.WriteByte('$')
			i++
		case next == '&':
			b.WriteString(input[loc[0]:loc[1]])
			i++
		case next == '`':
			b.WriteString(input[:loc[0]])
			i++
		case next == '\'':
			b.WriteString(input[loc[1]:])
			i++
		case next >= '0' && next <= '9':
			if i+2 < len(tmpl) && tmpl[i+2] >= '0' && tmpl[i+2] <= '9' {
				n, _ := strconv.Atoi(tmpl[i+1 : i+3])
				if n >= 1 && n <= groups {
					b.WriteString(group(n))
					i += 2
					continue
				}
			}
			n := int(next - '0')
			if n >= 1 && n <= groups {
				b.WriteString(group(n))
				i++
				continue
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Split implements String.prototype.split with a regular expression: the
// text of capturing groups is spliced into the result.
func Split(re *regexp.Regexp, s string) []string {
	if s == "" {
		if re.MatchString("") {
			return []string{}
		}
		return []string{""}
	}
	var parts []string
	last := 0
	for _, loc := range re.FindAllStringSubmatchIndex(s, -1) {
		if loc[1] == loc[0] && (loc[0] == 0 || loc[0] >= len(s)) {
			continue
		}
		parts = append(parts, s[last:loc[0]])
		for g := 1; g < len(loc)/2; g++ {
			if loc[2*g] >= 0 {
				parts = append(parts, s[loc[2*g]:loc[2*g+1]])
			} else {
				parts = append(parts, "")
			}
		}
		last = loc[1]
	}
	return append(parts, s[last:])
}

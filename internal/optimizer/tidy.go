package optimizer

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/strutil"
	"github.com/clean-css/clean-css/internal/token"
)

var (
	caseAttributePattern       = jsutil.MustCompile(`[\s"'][iI]\s*\]`, "")
	caseRestorePattern         = regexp.MustCompile(`([\d\w])([iI])\]`)
	doubleQuoteCasePattern     = regexp.MustCompile(`="([a-zA-Z][a-zA-Z\d\-_]+)"([iI])`)
	doubleQuotePattern         = jsutil.MustCompile(`="([a-zA-Z][a-zA-Z\d\-_]+)"(\s|\])`, "g")
	htmlCommentPattern         = jsutil.MustCompile(`^(?:(?:<!--|-->)\s*)+`, "")
	singleQuoteCasePattern     = regexp.MustCompile(`='([a-zA-Z][a-zA-Z\d\-_]+)'([iI])`)
	singleQuotePattern         = jsutil.MustCompile(`='([a-zA-Z][a-zA-Z\d\-_]+)'(\s|\])`, "g")
	adjacentNavPattern         = jsutil.MustCompile(`\+nav(\S|$)`, "")
	asteriskPrefixPattern      = regexp.MustCompile(`\*([:#.\[])`)
	plusHTMLHackPattern        = regexp.MustCompile(`^(:first-child)?\+html`)
	pseudoClassesWithSelectors = []string{":current", ":future", ":has", ":host", ":host-context", ":is", ":not", ":past", ":where"}
)

func isRelation(character string) bool {
	return character == ">" || character == "+" || character == "~"
}

func isWhitespace(character string) bool {
	r, _ := utf8.DecodeRuneInString(character)
	return character != "" && jsutil.IsSpace(r)
}

// characters splits s into characters; an astral character counts twice,
// as it does for the original, the second time as an empty string.
func characters(s string) []string {
	var chars []string
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		chars = append(chars, s[i:i+size])
		if r > 0xffff {
			chars = append(chars, "")
		}
		i += size
	}
	return chars
}

func hasInvalidCharacters(value string) bool {
	isEscaped := false
	isQuote := false

	for i, character := range characters(value) {
		switch {
		case isEscaped:
			// continue as always
		case character == "'" || character == `"`:
			isQuote = !isQuote
		case !isQuote && (character == "}" || character == "!" || character == "<" || character == ";"):
			return true
		case !isQuote && i == 0 && isRelation(character):
			return true
		}
		isEscaped = character == `\`
	}
	return false
}

func isPseudoClassWithSelectors(value string, colonPosition int) bool {
	end := strings.Index(value[colonPosition:], "(")
	if end > -1 {
		end += colonPosition
	}
	return slices.Contains(pseudoClassesWithSelectors, substring(value, colonPosition, end))
}

func removeWhitespace(value string, format *options.Format) string {
	var stripped []string
	var isEscaped, wasEscaped, isSingleQuoted, isDoubleQuoted, isAttribute, isSpaceAwarePseudoClass bool
	roundBracketLevel := 0
	wasComma := false
	wasRelation := false
	wasWhitespace := false
	withCaseAttribute := caseAttributePattern.MatchString(value)
	spaceAroundRelation := format != nil && format.AllowsSpace(options.AroundSelectorRelation)

	pop := func() {
		if len(stripped) > 0 {
			stripped = stripped[:len(stripped)-1]
		}
	}

	chars := characters(value)
	offset := 0
	for i, character := range chars {
		previous := ""
		if i > 0 {
			previous = chars[i-1]
		}
		isNewLineNix := character == "\n"
		isNewLineWin := character == "\n" && previous == "\r"
		isQuoted := isSingleQuoted || isDoubleQuoted
		relation := !isAttribute && !isEscaped && roundBracketLevel == 0 && isRelation(character)
		whitespace := isWhitespace(character)
		if roundBracketLevel == 1 && character == ")" {
			isSpaceAwarePseudoClass = false
		} else {
			isSpaceAwarePseudoClass = isSpaceAwarePseudoClass ||
				roundBracketLevel == 0 && character == ":" && isPseudoClassWithSelectors(value, offset)
		}

		switch {
		case wasEscaped && isQuoted && isNewLineWin:
			// swallow escaped new windows lines in comments
			pop()
			pop()
		case isEscaped && isQuoted && isNewLineNix:
			// swallow escaped new *nix lines in comments
			pop()
		case isEscaped:
			stripped = append(stripped, character)
		case character == "[" && !isQuoted:
			stripped = append(stripped, character)
			isAttribute = true
		case character == "]" && !isQuoted:
			stripped = append(stripped, character)
			isAttribute = false
		case character == "(" && !isQuoted:
			stripped = append(stripped, character)
			roundBracketLevel++
		case character == ")" && !isQuoted:
			stripped = append(stripped, character)
			roundBracketLevel--
		case character == "'" && !isQuoted:
			stripped = append(stripped, character)
			isSingleQuoted = true
		case character == `"` && !isQuoted:
			stripped = append(stripped, character)
			isDoubleQuoted = true
		case character == "'" && isQuoted:
			stripped = append(stripped, character)
			isSingleQuoted = false
		case character == `"` && isQuoted:
			stripped = append(stripped, character)
			isDoubleQuoted = false
		case whitespace && wasRelation && !spaceAroundRelation:
			// the original skips the bookkeeping below for this character
			offset += len(character)
			continue
		case !whitespace && wasRelation && spaceAroundRelation:
			stripped = append(stripped, " ", character)
		case whitespace && !wasWhitespace && wasComma && roundBracketLevel > 0 && isSpaceAwarePseudoClass:
			// skip space
		case whitespace && !wasWhitespace && roundBracketLevel > 0 && isSpaceAwarePseudoClass:
			stripped = append(stripped, character)
		case whitespace && (isAttribute || roundBracketLevel > 0) && !isQuoted:
			// skip space
		case whitespace && wasWhitespace && !isQuoted:
			// skip extra space
		case (isNewLineWin || isNewLineNix) && (isAttribute || roundBracketLevel > 0) && isQuoted:
			// skip newline
		case relation && wasWhitespace && !spaceAroundRelation:
			pop()
			stripped = append(stripped, character)
		case relation && !wasWhitespace && spaceAroundRelation:
			stripped = append(stripped, " ", character)
		case whitespace:
			stripped = append(stripped, " ")
		default:
			stripped = append(stripped, character)
		}

		offset += len(character)
		wasEscaped = isEscaped
		isEscaped = character == `\`
		wasRelation = relation
		wasWhitespace = whitespace
		wasComma = character == ","
	}

	result := strings.Join(stripped, "")
	if withCaseAttribute {
		return caseRestorePattern.ReplaceAllString(result, "$1 $2]")
	}
	return result
}

func removeQuotes(value string) string {
	if !strings.ContainsAny(value, `'"`) {
		return value
	}
	value = singleQuoteCasePattern.ReplaceAllString(value, "=$1 $2")
	value = jsutil.Replace(singleQuotePattern, value, "=$1$2", true)
	value = doubleQuoteCasePattern.ReplaceAllString(value, "=$1 $2")
	return jsutil.Replace(doubleQuotePattern, value, "=$1$2", true)
}

var pseudoClassReplacements = [][2]string{
	{"nth-child(1)", "first-child"},
	{"nth-of-type(1)", "first-of-type"},
	{"nth-of-type(even)", "nth-of-type(2n)"},
	{"nth-child(even)", "nth-child(2n)"},
	{"nth-of-type(2n+1)", "nth-of-type(odd)"},
	{"nth-child(2n+1)", "nth-child(odd)"},
	{"nth-last-child(1)", "last-child"},
	{"nth-last-of-type(1)", "last-of-type"},
	{"nth-last-of-type(even)", "nth-last-of-type(2n)"},
	{"nth-last-child(even)", "nth-last-child(2n)"},
	{"nth-last-of-type(2n+1)", "nth-last-of-type(odd)"},
	{"nth-last-child(2n+1)", "nth-last-child(odd)"},
}

func replacePseudoClasses(value string) string {
	for _, replacement := range pseudoClassReplacements {
		value = strings.Replace(value, replacement[0], replacement[1], 1)
	}
	return value
}

// TidyRules normalizes the selectors of a rule: whitespace, quotes and
// pseudo-classes are minified, invalid and repeated selectors are dropped
// with warnings, and IE hacks are dropped when removeUnsupported is set.
func TidyRules(rules []*token.Token, removeUnsupported, adjacentSpace bool, format *options.Format, ctx *Context) []*token.Token {
	list := []*token.Token{}
	var repeated []string

	for _, rule := range rules {
		reduced := jsutil.ReplaceFunc(htmlCommentPattern, rule.Value, false, func(m jsutil.Match) string {
			ctx.Warn("HTML comment '" + m.Text() + "' at " + positionOf(rule) + ". Removing.")
			return ""
		})

		if hasInvalidCharacters(reduced) {
			ctx.Warn("Invalid selector '" + rule.Value + "' at " + positionOf(rule) + ". Ignoring.")
			continue
		}

		reduced = removeWhitespace(reduced, format)
		reduced = removeQuotes(reduced)

		if adjacentSpace && strings.Index(reduced, "nav") > 0 {
			reduced = jsutil.Replace(adjacentNavPattern, reduced, "+ nav$1", false)
		}

		if removeUnsupported && strings.Contains(reduced, "*+html ") {
			continue
		}

		if removeUnsupported && strings.Contains(reduced, "*:first-child+html ") {
			continue
		}

		if strings.Contains(reduced, "*") {
			reduced = asteriskPrefixPattern.ReplaceAllString(reduced, "$1")
			reduced = jsutil.Replace(plusHTMLHackPattern, reduced, "*$1+html", false)
		}

		if slices.Contains(repeated, reduced) {
			continue
		}

		reduced = replacePseudoClasses(reduced)

		rule.Value = reduced
		repeated = append(repeated, reduced)
		list = append(list, rule)
	}

	if len(list) == 1 && list[0].Value == "" {
		ctx.Warn("Empty selector '" + list[0].Value + "' at " + positionOf(list[0]) + ". Ignoring.")
		list = []*token.Token{}
	}

	return list
}

func standardSorter(scope1, scope2 *token.Token) float64 {
	if jsutil.CompareUTF16(scope1.Value, scope2.Value) > 0 {
		return 1
	}
	return -1
}

func naturalSorter(scope1, scope2 *token.Token) float64 {
	return float64(strutil.NaturalCompare(scope1.Value, scope2.Value))
}

// SortSelectors sorts the selectors of a rule in place with the
// selectorsSortingMethod option ("standard", "natural", "none" or false); any
// other method yields nil.
func SortSelectors(selectors []*token.Token, method any) []*token.Token {
	switch {
	case method == "natural":
		jsutil.Sort(selectors, naturalSorter)
		return selectors
	case method == "standard":
		jsutil.Sort(selectors, standardSorter)
		return selectors
	case method == "none" || method == false:
		return selectors
	}
	return nil
}

var (
	compactBlockPattern      = jsutil.MustCompile(`^@media\W`, "")
	quoteRemovalBlockPattern = jsutil.MustCompile(`^@(?:keyframes|-moz-keyframes|-o-keyframes|-webkit-keyframes)\W`, "")
	blockLineBreakPattern    = regexp.MustCompile(`\n|\r\n`)
	blockWhitespacePattern   = jsutil.MustCompile(`\s+`, "g")
	blockSeparatorPattern    = regexp.MustCompile(`(,|:|\() `)
	blockSingleQuotedPattern = regexp.MustCompile(`'([a-zA-Z][a-zA-Z\d\-_]+)'`)
	blockDoubleQuotedPattern = regexp.MustCompile(`"([a-zA-Z][a-zA-Z\d\-_]+)"`)
)

// tidyBlock minifies the scopes of a nested block such as @media.
func tidyBlock(values []*token.Token, spaceAfterClosingBrace bool) []*token.Token {
	for i := len(values) - 1; i >= 0; i-- {
		value := values[i]
		withoutSpaceAfterClosingBrace := !spaceAfterClosingBrace && compactBlockPattern.MatchString(value.Value)
		withoutQuotes := quoteRemovalBlockPattern.MatchString(value.Value)

		text := blockLineBreakPattern.ReplaceAllLiteralString(value.Value, " ")
		text = blockWhitespacePattern.ReplaceAllLiteralString(text, " ")
		text = blockSeparatorPattern.ReplaceAllString(text, "$1")
		text = strings.ReplaceAll(text, " )", ")")

		if withoutQuotes {
			text = jsutil.Replace(blockSingleQuotedPattern, text, "$1", false)
			text = jsutil.Replace(blockDoubleQuotedPattern, text, "$1", false)
		}

		if withoutSpaceAfterClosingBrace {
			text = strings.ReplaceAll(text, ") ", ")")
		}

		value.Value = text
	}
	return values
}

var (
	atRuleURLWhitespacePattern     = jsutil.MustCompile(`url\(\s+`, "g")
	atRuleClosingWhitespacePattern = jsutil.MustCompile(`\s+\)`, "g")
)

// tidyAtRule collapses the whitespace of an at-rule such as @import.
func tidyAtRule(value string) string {
	value = blockWhitespacePattern.ReplaceAllLiteralString(value, " ")
	value = atRuleURLWhitespacePattern.ReplaceAllLiteralString(value, "url(")
	value = atRuleClosingWhitespacePattern.ReplaceAllLiteralString(value, ")")
	return jsutil.Trim(value)
}

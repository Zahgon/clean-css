package level2

import (
	"regexp"
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/strutil"
)

var (
	deepSelectorPattern    = regexp.MustCompile(`/deep/`)
	vendorPrefixedPseudo   = regexp.MustCompile(`:(-moz-|-ms-|-o-|-webkit-)`)
	relationPattern        = regexp.MustCompile(`[>+~]`)
	pseudoWithArguments    = []string{":dir", ":lang", ":not", ":nth-child", ":nth-last-child", ":nth-last-of-type", ":nth-of-type"}
	unmixablePseudoClass   = []string{":after", ":before", ":first-letter", ":first-line", ":lang"}
	unmixablePseudoElement = []string{"::after", "::before", "::first-letter", "::first-line"}
)

// mergeability holds the compatibility settings that decide which selectors
// can be merged with others.
type mergeability struct {
	pseudoClasses         []string
	pseudoElements        []string
	multiplePseudoMerging bool
}

func mergeabilityOf(ctx *optimizer.Context) mergeability {
	compatibility := ctx.Options.Compatibility
	return mergeability{
		pseudoClasses:         compatibility.Strings("selectors", "mergeablePseudoClasses"),
		pseudoElements:        compatibility.Strings("selectors", "mergeablePseudoElements"),
		multiplePseudoMerging: compatibility.Bool("selectors", "multiplePseudoMerging"),
	}
}

// isMergeable reports whether a serialized selector list can be merged with
// other rules: no /deep/, vendor-prefixed or unsupported pseudo-classes.
func (m mergeability) isMergeable(selector string) bool {
	for _, single := range strutil.Split(selector, ",") {
		if single == "" ||
			deepSelectorPattern.MatchString(single) ||
			vendorPrefixedPseudo.MatchString(single) ||
			strings.Contains(single, ":") && !m.areMergeable(single, extractPseudoFrom(single)) {
			return false
		}
	}
	return true
}

type quoteLevel int

const (
	levelRoot quoteLevel = iota
	levelDoubleQuote
	levelSingleQuote
)

func extractPseudoFrom(selector string) []string {
	var list []string
	var buffer strings.Builder
	level := levelRoot
	roundBracketLevel := 0
	isEscaped := false
	isPseudo := false
	wasColon := false

	for i := 0; i < len(selector); i++ {
		character := selector[i]
		isRelation := !isEscaped && relationPattern.MatchString(string(character))
		isQuoted := level == levelDoubleQuote || level == levelSingleQuote

		switch {
		case isEscaped:
			buffer.WriteByte(character)
		case character == '"' && level == levelRoot:
			buffer.WriteByte(character)
			level = levelDoubleQuote
		case character == '"' && level == levelDoubleQuote:
			buffer.WriteByte(character)
			level = levelRoot
		case character == '\'' && level == levelRoot:
			buffer.WriteByte(character)
			level = levelSingleQuote
		case character == '\'' && level == levelSingleQuote:
			buffer.WriteByte(character)
			level = levelRoot
		case isQuoted:
			buffer.WriteByte(character)
		case character == '(':
			buffer.WriteByte(character)
			roundBracketLevel++
		case character == ')' && roundBracketLevel == 1 && isPseudo:
			buffer.WriteByte(character)
			list = append(list, buffer.String())
			roundBracketLevel--
			buffer.Reset()
			isPseudo = false
		case character == ')':
			buffer.WriteByte(character)
			roundBracketLevel--
		case character == ':' && roundBracketLevel == 0 && isPseudo && !wasColon:
			list = append(list, buffer.String())
			buffer.Reset()
			buffer.WriteByte(character)
		case character == ':' && roundBracketLevel == 0 && !wasColon:
			buffer.Reset()
			buffer.WriteByte(character)
			isPseudo = true
		case character == ' ' && roundBracketLevel == 0 && isPseudo:
			list = append(list, buffer.String())
			buffer.Reset()
			isPseudo = false
		case isRelation && roundBracketLevel == 0 && isPseudo:
			list = append(list, buffer.String())
			buffer.Reset()
			isPseudo = false
		default:
			buffer.WriteByte(character)
		}

		isEscaped = character == '\\'
		wasColon = character == ':'
	}

	if buffer.Len() > 0 && isPseudo {
		list = append(list, buffer.String())
	}

	return list
}

func (m mergeability) areMergeable(selector string, matches []string) bool {
	return m.areAllowed(matches) &&
		needArguments(matches) &&
		(len(matches) < 2 || !someIncorrectlyChained(selector, matches)) &&
		(len(matches) < 2 || m.multiplePseudoMerging && allMixable(matches))
}

func pseudoName(match string) string {
	if at := strings.Index(match, "("); at > -1 {
		return match[:at]
	}
	return match
}

func (m mergeability) areAllowed(matches []string) bool {
	for _, match := range matches {
		name := pseudoName(match)
		if !slices.Contains(m.pseudoClasses, name) && !slices.Contains(m.pseudoElements, name) {
			return false
		}
	}
	return true
}

func needArguments(matches []string) bool {
	for _, match := range matches {
		hasArguments := strings.Contains(match, "(")
		takesArguments := slices.Contains(pseudoWithArguments, pseudoName(match))
		if hasArguments != takesArguments {
			return false
		}
	}
	return true
}

func someIncorrectlyChained(selector string, matches []string) bool {
	positionInSelector := 0

	for i, match := range matches {
		if i+1 >= len(matches) || matches[i+1] == "" {
			break
		}
		nextMatch := matches[i+1]

		matchAt := indexFrom(selector, match, positionInSelector)
		nextMatchAt := indexFrom(selector, match, matchAt+1)
		positionInSelector = nextMatchAt
		areChained := matchAt+len(match) == nextMatchAt

		if areChained && (pseudoName(match) != ":not" || pseudoName(nextMatch) != ":not") {
			return true
		}
	}

	return false
}

func allMixable(matches []string) bool {
	unmixable := 0
	for _, match := range matches {
		if strings.HasPrefix(match, "::") {
			if slices.Contains(unmixablePseudoElement, match) {
				unmixable++
			}
		} else if slices.Contains(unmixablePseudoClass, match) {
			unmixable++
		}
		if unmixable > 1 {
			return false
		}
	}
	return true
}

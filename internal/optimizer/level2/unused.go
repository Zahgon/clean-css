package level2

import (
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
)

var (
	animationNameRegex = regexp.MustCompile(`^(-moz-|-o-|-webkit-)?animation-name$`)
	animationRegex     = regexp.MustCompile(`^(-moz-|-o-|-webkit-)?animation$`)
	keyframeRegex      = regexp.MustCompile(`^@(-moz-|-o-|-webkit-)?keyframes `)
	importantRegex     = jsutil.MustCompile(`\s{0,31}!important$`, "")
)

// atRuleSet maps an at-rule name (a font family, keyframes name, counter
// style or namespace prefix) to the tokens that define it.
type atRuleSet = jsutil.Object[[]*token.Token]

func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

// normalize strips matching quotes around a name (when the text inside has
// no line break) and a trailing !important.
func normalize(value string) string {
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] &&
		!strings.ContainsFunc(value[1:len(value)-1], isLineTerminator) {
		value = value[1 : len(value)-1]
	}
	return jsutil.Replace(importantRegex, value, "", false)
}

// secondWord returns item 1 of value split at spaces, "undefined" when there
// is none, as the original's property lookup does.
func secondWord(value string) string {
	parts := strings.Split(value, " ")
	if len(parts) < 2 {
		return "undefined"
	}
	return parts[1]
}

type matcher func(t *token.Token, atRules *atRuleSet)

type marker func(atRules *atRuleSet) func(t *token.Token, ctx *optimizer.Context)

// removeUnusedAtRules empties @counter-style, @font-face, @keyframes and
// @namespace rules that no rule refers to.
func removeUnusedAtRules(tokens []*token.Token, ctx *optimizer.Context) {
	removeUnusedAtRule(tokens, matchCounterStyle, markCounterStylesAsUsed, ctx)
	removeUnusedAtRule(tokens, matchFontFace, markFontFacesAsUsed, ctx)
	removeUnusedAtRule(tokens, matchKeyframe, markKeyframesAsUsed, ctx)
	removeUnusedAtRule(tokens, matchNamespace, markNamespacesAsUsed, ctx)
}

func removeUnusedAtRule(tokens []*token.Token, match matcher, mark marker, ctx *optimizer.Context) {
	atRules := jsutil.NewObject[[]*token.Token]()
	for _, t := range tokens {
		match(t, atRules)
	}
	if atRules.Len() == 0 {
		return
	}

	markUsedAtRules(tokens, mark(atRules), ctx)

	for _, name := range atRules.Keys() {
		atRuleTokens, _ := atRules.Get(name)
		for _, t := range atRuleTokens {
			if t.Kind == token.AtRule {
				t.Value = ""
			} else {
				t.Body = []*token.Token{}
			}
		}
	}
}

func markUsedAtRules(tokens []*token.Token, mark func(*token.Token, *optimizer.Context), ctx *optimizer.Context) {
	for _, t := range tokens {
		switch t.Kind {
		case token.Rule:
			mark(t, ctx)
		case token.NestedBlock:
			markUsedAtRules(t.Body, mark, ctx)
		}
	}
}

func addAtRule(atRules *atRuleSet, name string, t *token.Token) {
	list, _ := atRules.Get(name)
	atRules.Set(name, append(list, t))
}

func matchCounterStyle(t *token.Token, atRules *atRuleSet) {
	if t.Kind == token.AtRuleBlock && len(t.Scopes) > 0 && strings.HasPrefix(t.Scopes[0].Value, "@counter-style") {
		addAtRule(atRules, secondWord(t.Scopes[0].Value), t)
	}
}

// componentValues wraps a shorthand declaration, breaks it up and returns the
// texts of the values of one of its components (read before the declaration
// is put back, which restores !important onto shared value tokens).
func componentValues(property *token.Token, component int, ctx *optimizer.Context) []string {
	wrapped := optimizer.WrapSingle(property)
	populateComponents([]*optimizer.Property{wrapped}, ctx.Validator, &ctx.Warnings)
	var values []string
	if component < len(wrapped.Components) {
		for _, value := range wrapped.Components[component].Value {
			values = append(values, value.Value)
		}
	}
	optimizer.RestoreFromOptimizing([]*optimizer.Property{wrapped}, nil)
	return values
}

func markCounterStylesAsUsed(atRules *atRuleSet) func(*token.Token, *optimizer.Context) {
	return func(t *token.Token, ctx *optimizer.Context) {
		for _, property := range t.Body {
			if property.Kind != token.Property {
				continue
			}

			if property.Name.Value == "list-style" {
				wrapped := optimizer.WrapSingle(property)
				populateComponents([]*optimizer.Property{wrapped}, ctx.Validator, &ctx.Warnings)
				if len(wrapped.Components) > 0 && len(wrapped.Components[0].Value) > 0 &&
					atRules.Has(wrapped.Components[0].Value[0].Value) {
					if value, ok := firstValue(property); ok {
						atRules.Delete(value)
					}
				}
				optimizer.RestoreFromOptimizing([]*optimizer.Property{wrapped}, nil)
			}

			if value, ok := firstValue(property); ok && property.Name.Value == "list-style-type" && atRules.Has(value) {
				atRules.Delete(value)
			}
		}
	}
}

func matchFontFace(t *token.Token, atRules *atRuleSet) {
	if t.Kind != token.AtRuleBlock || len(t.Scopes) == 0 || t.Scopes[0].Value != "@font-face" {
		return
	}
	for _, property := range t.Body {
		if property.Kind == token.Property && property.Name.Value == "font-family" {
			value, _ := firstValue(property)
			addAtRule(atRules, normalize(strings.ToLower(value)), t)
			break
		}
	}
}

func markFontFacesAsUsed(atRules *atRuleSet) func(*token.Token, *optimizer.Context) {
	return func(t *token.Token, ctx *optimizer.Context) {
		for _, property := range t.Body {
			if property.Kind != token.Property {
				continue
			}

			if property.Name.Value == "font" {
				for _, value := range componentValues(property, 6, ctx) {
					if name := normalize(strings.ToLower(value)); atRules.Has(name) {
						atRules.Delete(name)
					}
				}
			}

			if property.Name.Value == "font-family" {
				for _, value := range property.Values {
					if name := normalize(strings.ToLower(value.Value)); atRules.Has(name) {
						atRules.Delete(name)
					}
				}
			}
		}
	}
}

func matchKeyframe(t *token.Token, atRules *atRuleSet) {
	if t.Kind == token.NestedBlock && len(t.Scopes) > 0 && keyframeRegex.MatchString(t.Scopes[0].Value) {
		addAtRule(atRules, secondWord(t.Scopes[0].Value), t)
	}
}

func markKeyframesAsUsed(atRules *atRuleSet) func(*token.Token, *optimizer.Context) {
	return func(t *token.Token, ctx *optimizer.Context) {
		for _, property := range t.Body {
			if property.Kind != token.Property {
				continue
			}

			if animationRegex.MatchString(property.Name.Value) {
				for _, value := range componentValues(property, 7, ctx) {
					if atRules.Has(value) {
						atRules.Delete(value)
					}
				}
			}

			if animationNameRegex.MatchString(property.Name.Value) {
				for _, value := range property.Values {
					if atRules.Has(value.Value) {
						atRules.Delete(value.Value)
					}
				}
			}
		}
	}
}

func matchNamespace(t *token.Token, atRules *atRuleSet) {
	if t.Kind == token.AtRule && strings.HasPrefix(t.Value, "@namespace") {
		addAtRule(atRules, secondWord(t.Value), t)
	}
}

func markNamespacesAsUsed(atRules *atRuleSet) func(*token.Token, *optimizer.Context) {
	namespacePattern := compileNamespacePattern(strings.Join(atRules.Keys(), `\||`) + `\|`)

	return func(t *token.Token, _ *optimizer.Context) {
		if namespacePattern == nil {
			return
		}
		for _, scope := range t.Scopes {
			for _, match := range namespacePattern.FindAllString(scope.Value, -1) {
				if prefix := match[:len(match)-1]; atRules.Has(prefix) {
					atRules.Delete(prefix)
				}
			}
		}
	}
}

// compileNamespacePattern compiles the alternation of namespace prefixes; the
// prefixes are used as written, so one that is not a valid expression
// matches nothing.
func compileNamespacePattern(source string) *regexp.Regexp {
	pattern, err := regexp.Compile(source)
	if err != nil {
		return nil
	}
	return pattern
}

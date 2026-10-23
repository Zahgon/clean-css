package optimizer

import (
	"math"
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

var (
	charsetPattern              = regexp.MustCompile(`(?i)^@charset`)
	importPrefixPattern         = regexp.MustCompile(`(?i)^@import`)
	variablePropertyNamePattern = jsutil.MustCompile(`^--\S+$`, "")
	propertyNamePattern         = regexp.MustCompile(`^(?:-chrome-|-[\w-]+\w|\w[\w-]+\w|\w{1,})$`)
)

func isLegacyFilter(property *Property) bool {
	if property.Name != "filter" && property.Name != "-ms-filter" {
		return false
	}
	value := property.Value[0].Value
	return strings.Contains(value, "progid") || strings.HasPrefix(value, "alpha") || strings.HasPrefix(value, "chroma")
}

// blockPosition formats the position of the first declaration of a property
// block value, the place the original reports for it.
func blockPosition(block *token.Token) string {
	if len(block.Body) == 0 {
		return "undefined"
	}
	first := block.Body[0]
	if first.Kind == token.Property {
		return positionOf(first.Name)
	}
	return "undefined"
}

func optimizeBody(rule []*token.Token, properties *[]*token.Token, ctx *Context) {
	opts := ctx.Options
	compatibility := opts.Compatibility
	serializedRule := writer.Rules(rule)
	wrapped := WrapAll(properties, nil)

	for _, property := range wrapped {
		name := property.Name
		descriptor, known := Lookup(name)
		var propertyOptimizer PropertyOptimizer
		valueOptimizers := []ValueOptimizer{optimizeWhitespace}
		if known {
			propertyOptimizer = descriptor.PropertyOptimizer
			if descriptor.ValueOptimizers != nil {
				valueOptimizers = descriptor.ValueOptimizers
			}
		}

		isVariable := variablePropertyNamePattern.MatchString(name)
		if isVariable {
			valueOptimizers = []ValueOptimizer{optimizeWhitespace}
			if len(opts.Runtime.VariableOptimizers) > 0 {
				valueOptimizers = nil
				for _, optimizerName := range opts.Runtime.VariableOptimizers {
					optimizer, ok := ValueOptimizerByName(optimizerName)
					if !ok {
						optimizer = noopValueOptimizer
					}
					valueOptimizers = append(valueOptimizers, optimizer)
				}
			}
		}

		if !isVariable && !propertyNamePattern.MatchString(name) {
			ctx.Warn("Invalid property name '" + name + "' at " + namePositionOf(property) + ". Ignoring.")
			property.Unused = true
			continue
		}

		if len(property.Value) == 0 {
			ctx.Warn("Empty property '" + name + "' at " + namePositionOf(property) + ". Ignoring.")
			property.Unused = true
			continue
		}

		hack := property.HackKind()
		if property.Hack != nil &&
			((hack == HackAsterisk || hack == HackUnderscore) && !compatibility.Bool("properties", "iePrefixHack") ||
				hack == HackBackslash && !compatibility.Bool("properties", "ieSuffixHack") ||
				hack == HackBang && !compatibility.Bool("properties", "ieBangHack")) {
			property.Unused = true
			continue
		}

		if !compatibility.Bool("properties", "ieFilters") && isLegacyFilter(property) {
			property.Unused = true
			continue
		}

		if property.Block {
			optimizeBody(rule, &property.Value[0].Body, ctx)
			continue
		}

		for _, value := range property.Value {
			if value.Kind == token.PropertyBlock {
				property.Unused = true
				ctx.Warn("Invalid value token at " + blockPosition(value) + ". Ignoring.")
				break
			}

			text := value.Value
			if startsAsURL(text) && !ctx.Validator.IsUrl(text) {
				property.Unused = true
				ctx.Warn("Broken URL '" + text + "' at " + positionOf(value) + ". Ignoring.")
				break
			}

			for _, optimizer := range valueOptimizers {
				text = optimizer(name, text, opts)
			}
			value.Value = text
		}

		if propertyOptimizer != nil {
			propertyOptimizer(serializedRule, property, opts)
		}
	}

	RestoreFromOptimizing(wrapped, nil)
	RemoveUnused(wrapped)
	removeComments(properties, opts)
}

func removeComments(tokens *[]*token.Token, opts *options.Options) {
	for i := 0; i < len(*tokens); i++ {
		t := (*tokens)[i]
		if t.Kind != token.Comment {
			continue
		}

		optimizeComment(t, opts)

		if t.Value == "" {
			*tokens = append((*tokens)[:i:i], (*tokens)[i+1:]...)
			i--
		}
	}
}

// optimizeComment keeps special comments (/*! ... */) as the specialComments
// option allows and empties every other comment. The count of kept comments
// lives on the options, so it carries over to later minifications.
func optimizeComment(t *token.Token, opts *options.Options) {
	specialComments := levelOne(opts).Get("specialComments")
	if len(t.Value) > 2 && t.Value[2] == '!' &&
		(jsutil.LooseEqual(specialComments, "all") || jsutil.LessThan(float64(opts.Runtime.CommentsKept), specialComments)) {
		opts.Runtime.CommentsKept++
		return
	}
	t.Value = ""
}

func cleanupCharsets(tokens []*token.Token) []*token.Token {
	hasCharset := false

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Kind != token.AtRule {
			continue
		}
		if !charsetPattern.MatchString(t.Value) {
			continue
		}

		if hasCharset || !strings.Contains(t.Value, "@charset") {
			tokens = append(tokens[:i:i], tokens[i+1:]...)
			i--
		} else {
			hasCharset = true
			tokens = append(tokens[:i:i], tokens[i+1:]...)
			charset := &token.Token{Kind: token.AtRule, Value: charsetPattern.ReplaceAllLiteralString(t.Value, "@charset")}
			tokens = append([]*token.Token{charset}, tokens...)
		}
	}

	return tokens
}

func buildUnitPattern(opts *options.Options) *regexp.Regexp {
	units := []string{"px", "em", "ex", "cm", "mm", "in", "pt", "pc", "%"}
	for _, unit := range []string{"ch", "rem", "vh", "vm", "vmax", "vmin", "vw"} {
		if opts.Compatibility.Bool("units", unit) {
			units = append(units, unit)
		}
	}
	return jsutil.MustCompile(`(^|\s|\(|,)0(?:`+strings.Join(units, "|")+`)(\W|$)`, "g")
}

func buildPrecision(roundingPrecision any) options.Precision {
	precision := options.Precision{Units: map[string]options.UnitPrecision{}}
	var optimizable []string

	if o, ok := jsutil.AsObject(roundingPrecision); ok {
		for _, unit := range o.Keys() {
			value := o.Lookup(unit)
			if jsutil.LooseEqual(value, options.PrecisionOff) {
				continue
			}
			precision.Units[unit] = options.UnitPrecision{Value: value, Multiplier: math.Pow(10, jsutil.ToNumber(value))}
			optimizable = append(optimizable, unit)
		}
	}

	if len(optimizable) > 0 {
		alternatives := strings.Join(optimizable, "|")
		precision.Enabled = true
		precision.DecimalPointPattern = regexp.MustCompile(`(\d)\.($|` + alternatives + `)($|\W)`)
		precision.ZeroPattern = regexp.MustCompile(`(\d*)(\.\d+)(` + alternatives + `)`)
	}

	return precision
}

func buildVariableOptimizers(levelOptions options.Values) []string {
	list, _ := levelOptions.Get("variableValueOptimizers").([]any)
	names := make([]string, len(list))
	for i, item := range list {
		names[i] = jsutil.ToString(item)
	}
	return names
}

func prepare(opts *options.Options) {
	if opts.Runtime.Prepared {
		return
	}
	levelOptions := levelOne(opts)
	opts.Runtime.UnitsPattern = buildUnitPattern(opts)
	opts.Runtime.Precision = buildPrecision(levelOptions.Get("roundingPrecision"))
	opts.Runtime.VariableOptimizers = buildVariableOptimizers(levelOptions)
	opts.Runtime.Prepared = true
}

func isEmptyToken(t *token.Token) bool {
	switch t.Kind {
	case token.Rule, token.NestedBlock, token.AtRuleBlock:
		return len(t.Scopes) == 0 || len(t.Body) == 0
	}
	return t.Value == ""
}

// Level1 applies the level 1 optimizations to a list of tokens: selectors,
// at-rules and values are minified one by one and comments are handled by
// the specialComments option. It returns the remaining tokens.
func Level1(tokens []*token.Token, ctx *Context) []*token.Token {
	opts := ctx.Options
	levelOptions := levelOne(opts)
	ie7Hack := opts.Compatibility.Bool("selectors", "ie7Hack")
	adjacentSpace := opts.Compatibility.Bool("selectors", "adjacentSpace")
	spaceAfterClosingBrace := opts.Compatibility.Bool("properties", "spaceAfterClosingBrace")
	mayHaveCharset := false
	afterRules := false

	prepare(opts)

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		switch t.Kind {
		case token.AtRule:
			if importPrefixPattern.MatchString(t.Value) && afterRules {
				t.Value = ""
			}
			if levelOptions.Bool("tidyAtRules") {
				t.Value = tidyAtRule(t.Value)
			}
			mayHaveCharset = true
		case token.AtRuleBlock:
			optimizeBody(t.Scopes, &t.Body, ctx)
			afterRules = true
		case token.NestedBlock:
			if levelOptions.Bool("tidyBlockScopes") {
				t.Scopes = tidyBlock(t.Scopes, spaceAfterClosingBrace)
			}
			t.Body = Level1(t.Body, ctx)
			afterRules = true
		case token.Comment:
			optimizeComment(t, opts)
		case token.Rule:
			if levelOptions.Bool("tidySelectors") {
				t.Scopes = TidyRules(t.Scopes, !ie7Hack, adjacentSpace, opts.Format, ctx)
			}
			if len(t.Scopes) > 1 {
				t.Scopes = SortSelectors(t.Scopes, levelOptions.Get("selectorsSortingMethod"))
			}
			optimizeBody(t.Scopes, &t.Body, ctx)
			afterRules = true
		}

		if t.Kind == token.Comment && t.Value == "" || levelOptions.Bool("removeEmpty") && isEmptyToken(t) {
			tokens = append(tokens[:i:i], tokens[i+1:]...)
			i--
		}
	}

	if levelOptions.Bool("cleanupCharsets") && mayHaveCharset {
		tokens = cleanupCharsets(tokens)
	}

	return tokens
}

// Package writer serializes token trees: Styles writes the final output with
// the format options, and the one-time serializers (All, Body, Property,
// Rules, Value) write compact text that the optimizers compare.
package writer

import (
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
)

// context is the state of one serialization.
type context struct {
	output                 []string
	format                 *options.Format
	indentBy               float64
	indentWith             string
	spaceAfterClosingBrace bool
	wraps                  bool
	line                   int
	column                 int
}

func (c *context) store(value string) {
	if c.wraps {
		if jsutil.LessThan(c.format.WrapAt, float64(c.column+jsutil.Len16(value))) {
			c.track(c.format.BreakWith)
			c.output = append(c.output, c.format.BreakWith)
		}
		c.track(value)
	}
	c.output = append(c.output, value)
}

func (c *context) track(value string) {
	parts := strings.Split(value, "\n")
	c.line += len(parts) - 1
	if len(parts) > 1 {
		c.column = 0
	} else {
		c.column += jsutil.Len16(parts[0])
	}
}

func (c *context) String() string {
	return strings.Join(c.output, "")
}

func supportsAfterClosingBrace(property *token.Token) bool {
	name := property.Name.Value
	return name == "background" || name == "transform" || name == "src"
}

// valueAt returns the text of value i of a property, and whether it exists.
func valueAt(property *token.Token, i int) (string, bool) {
	if i < 0 || i >= len(property.Values) {
		return "", false
	}
	return property.Values[i].Value, true
}

func afterClosingBrace(property *token.Token, i int) bool {
	value, _ := valueAt(property, i)
	return strings.HasSuffix(value, ")")
}

func isValue(property *token.Token, i int, marker string) bool {
	value, ok := valueAt(property, i)
	return ok && value == marker
}

func inFilter(property *token.Token) bool {
	name := property.Name.Value
	return name == "filter" || name == "-ms-filter"
}

func (c *context) disallowsSpace(property *token.Token, i int) bool {
	return !c.spaceAfterClosingBrace && supportsAfterClosingBrace(property) && afterClosingBrace(property, i) ||
		isValue(property, i+1, "/") ||
		isValue(property, i, "/") ||
		isValue(property, i+1, ",") ||
		isValue(property, i, ",")
}

func (c *context) rules(tokens []*token.Token) {
	for i, scope := range tokens {
		c.store(scope.Value)
		if i < len(tokens)-1 {
			c.store(c.comma())
		}
	}
}

func (c *context) body(tokens []*token.Token) {
	lastPropertyAt := lastPropertyIndex(tokens)
	for i := range tokens {
		c.property(tokens, i, lastPropertyAt, true)
	}
}

func lastPropertyIndex(tokens []*token.Token) int {
	index := len(tokens) - 1
	for ; index >= 0; index-- {
		if tokens[index].Kind != token.Comment {
			break
		}
	}
	return index
}

// property writes the token at position of a body. lastPropertyAt is the
// index of the last token that is not a comment; canBeLast is false when the
// caller serializes a single property (the original passes true there, which
// is never strictly equal to an index).
func (c *context) property(tokens []*token.Token, position, lastPropertyAt int, canBeLast bool) {
	t := tokens[position]
	isPropertyBlock := t.Kind == token.Property && t.IsBlockProperty()

	var needsSemicolon bool
	if c.format != nil {
		needsSemicolon = jsutil.Truthy(c.format.SemicolonAfterLastProperty) || isPropertyBlock || position < lastPropertyAt
	} else {
		needsSemicolon = position < lastPropertyAt || isPropertyBlock
	}

	isLast := canBeLast && position == lastPropertyAt

	switch t.Kind {
	case token.AtRule:
		c.store(t.Value)
		c.store(c.semicolon(options.AfterProperty, false))
	case token.AtRuleBlock:
		c.rules(t.Scopes)
		c.store(c.openBrace(options.AfterRuleBegins, true))
		c.body(t.Body)
		c.store(c.closeBrace(options.AfterRuleEnds, false, isLast))
	case token.Comment:
		c.store(t.Value)
		c.store(c.breakFor(options.AfterComment) + c.indentWith)
	case token.Property:
		c.store(t.Name.Value)
		c.store(c.colon())
		if len(t.Values) > 0 {
			c.value(t)
		}
		if needsSemicolon {
			c.store(c.semicolon(options.AfterProperty, isLast))
		} else {
			c.store("")
		}
	case token.Raw:
		c.store(t.Value)
	}
}

func (c *context) value(property *token.Token) {
	if property.IsBlockProperty() {
		c.store(c.openBrace(options.AfterBlockBegins, false))
		c.body(property.Values[0].Body)
		c.store(c.closeBrace(options.AfterBlockEnds, false, true))
		return
	}

	for i, v := range property.Values {
		c.store(v.Value)
		if i < len(property.Values)-1 && (inFilter(property) || !c.disallowsSpace(property, i)) {
			c.store(" ")
		}
	}
}

func (c *context) breakFor(where string) string {
	if c.format != nil {
		return c.format.Breaks[where]
	}
	return ""
}

func (c *context) allowsSpace(where string) bool {
	return c.format != nil && c.format.AllowsSpace(where)
}

func (c *context) indent(delta float64) {
	c.indentBy += delta
	c.indentWith = jsutil.Repeat(c.format.IndentWith, c.indentBy)
}

func (c *context) openBrace(where string, needsPrefixSpace bool) string {
	if c.format == nil {
		return "{"
	}
	c.indent(c.format.IndentBy)
	prefix := ""
	if needsPrefixSpace && c.allowsSpace(options.BeforeBlockBegins) {
		prefix = " "
	}
	return prefix + "{" + c.breakFor(where) + c.indentWith
}

func (c *context) closeBrace(where string, beforeBlockEnd, isLast bool) string {
	if c.format == nil {
		return "}"
	}
	c.indent(-c.format.IndentBy)
	before := c.breakFor(options.AfterProperty)
	if beforeBlockEnd {
		before = c.breakFor(options.BeforeBlockEnds)
	}
	after := ""
	if !isLast {
		after = c.breakFor(where) + c.indentWith
	}
	return before + c.indentWith + "}" + after
}

func (c *context) colon() string {
	if c.format != nil && c.allowsSpace(options.BeforeValue) {
		return ": "
	}
	return ":"
}

func (c *context) semicolon(where string, isLast bool) string {
	if c.format == nil || isLast {
		return ";"
	}
	return ";" + c.breakFor(where) + c.indentWith
}

func (c *context) comma() string {
	if c.format == nil {
		return ","
	}
	return "," + c.breakFor(options.BetweenSelectors) + c.indentWith
}

func (c *context) all(tokens []*token.Token) {
	for i, t := range tokens {
		isLast := i == len(tokens)-1

		switch t.Kind {
		case token.AtRule:
			c.store(t.Value)
			c.store(c.semicolon(options.AfterAtRule, isLast))
		case token.AtRuleBlock:
			c.rules(t.Scopes)
			c.store(c.openBrace(options.AfterRuleBegins, true))
			c.body(t.Body)
			c.store(c.closeBrace(options.AfterRuleEnds, false, isLast))
		case token.NestedBlock:
			c.rules(t.Scopes)
			c.store(c.openBrace(options.AfterBlockBegins, true))
			c.all(t.Body)
			c.store(c.closeBrace(options.AfterBlockEnds, true, isLast))
		case token.Comment:
			c.store(t.Value)
			c.store(c.breakFor(options.AfterComment) + c.indentWith)
		case token.Raw:
			c.store(t.Value)
		case token.Rule:
			c.rules(t.Scopes)
			c.store(c.openBrace(options.AfterRuleBegins, true))
			c.body(t.Body)
			c.store(c.closeBrace(options.AfterRuleEnds, false, isLast))
		}
	}
}

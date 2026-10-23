// Package token defines the token tree the tokenizer produces and the
// optimizers and writers work on.
package token

import "strconv"

// Kind identifies the type of a token.
type Kind int

// Token kinds.
const (
	// AtRule is a block-level at-rule without a body, e.g. @import or @charset.
	AtRule Kind = iota + 1
	// AtRuleBlock is an at-rule with declarations, e.g. @font-face{...}.
	AtRuleBlock
	// AtRuleBlockScope is the scope of an AtRuleBlock, e.g. @font-face.
	AtRuleBlockScope
	// Comment is a comment, e.g. /* comment */.
	Comment
	// NestedBlock is an at-rule holding rules, e.g. @media screen{...}.
	NestedBlock
	// NestedBlockScope is the scope of a NestedBlock, e.g. @media screen.
	NestedBlockScope
	// Property is a declaration, e.g. color:red.
	Property
	// PropertyBlock is a block value of a custom property, e.g. --var:{color:red}.
	PropertyBlock
	// PropertyName is the name of a declaration.
	PropertyName
	// PropertyValue is one value of a declaration.
	PropertyValue
	// Raw is text kept verbatim between clean-css ignore:start and ignore:end comments.
	Raw
	// Rule is a style rule, e.g. div > a{...}.
	Rule
	// RuleScope is one selector of a Rule.
	RuleScope
)

var kindNames = map[Kind]string{
	AtRule:           "at-rule",
	AtRuleBlock:      "at-rule-block",
	AtRuleBlockScope: "at-rule-block-scope",
	Comment:          "comment",
	NestedBlock:      "nested-block",
	NestedBlockScope: "nested-block-scope",
	Property:         "property",
	PropertyBlock:    "property-block",
	PropertyName:     "property-name",
	PropertyValue:    "property-value",
	Raw:              "raw",
	Rule:             "rule",
	RuleScope:        "rule-scope",
}

// String returns the name the original implementation uses for the kind.
func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "kind(" + strconv.Itoa(int(k)) + ")"
}

// Position is a place in a source: a 1-based line, a 0-based column counted
// in UTF-16 code units and the source name (empty for unnamed input).
type Position struct {
	Line   int
	Column int
	Source string
}

// String formats the position as source:line:column, or line:column for
// unnamed input.
func (p Position) String() string {
	location := strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Column)
	if p.Source != "" {
		return p.Source + ":" + location
	}
	return location
}

// Token is one node of the token tree. Which fields are used depends on Kind:
//
//   - leaf tokens (at-rules, comments, raw text, scopes, property names and
//     values) keep their text in Value and where it came from in Positions;
//   - Rule, NestedBlock and AtRuleBlock keep their scopes in Scopes and their
//     content in Body;
//   - PropertyBlock keeps its declarations in Body;
//   - Property keeps its name token in Name and its value tokens, or a single
//     PropertyBlock, in Values.
type Token struct {
	Kind      Kind
	Value     string
	Positions []Position
	Scopes    []*Token
	Body      []*Token
	Name      *Token
	Values    []*Token
}

// NewValue returns a property value token without a position.
func NewValue(value string) *Token {
	return &Token{Kind: PropertyValue, Value: value}
}

// NewProperty returns a property token named name with the given values.
func NewProperty(name string, values ...*Token) *Token {
	return &Token{Kind: Property, Name: &Token{Kind: PropertyName, Value: name}, Values: values}
}

// IsBlockProperty reports whether a property token holds a property block.
func (t *Token) IsBlockProperty() bool {
	return len(t.Values) > 0 && t.Values[0].Kind == PropertyBlock
}

// Clone returns a deep copy of the token, like cloning the nested arrays of
// the original representation.
func (t *Token) Clone() *Token {
	if t == nil {
		return nil
	}
	c := &Token{Kind: t.Kind, Value: t.Value}
	if t.Positions != nil {
		c.Positions = append([]Position(nil), t.Positions...)
	}
	c.Scopes = CloneAll(t.Scopes)
	c.Body = CloneAll(t.Body)
	c.Name = t.Name.Clone()
	c.Values = CloneAll(t.Values)
	return c
}

// CloneAll deep-copies a list of tokens; a nil list stays nil.
func CloneAll(tokens []*Token) []*Token {
	if tokens == nil {
		return nil
	}
	cloned := make([]*Token, len(tokens))
	for i, t := range tokens {
		cloned[i] = t.Clone()
	}
	return cloned
}

// Copy returns a shallow copy of a token list (Array.prototype.slice).
func Copy(tokens []*Token) []*Token {
	return append([]*Token{}, tokens...)
}

// Marker characters with a special meaning to the tokenizer and optimizers.
const (
	Asterisk           = '*'
	At                 = '@'
	BackSlash          = '\\'
	CarriageReturn     = '\r'
	CloseCurlyBracket  = '}'
	CloseRoundBracket  = ')'
	CloseSquareBracket = ']'
	Colon              = ':'
	Comma              = ','
	DoubleQuote        = '"'
	Exclamation        = '!'
	ForwardSlash       = '/'
	NewLineNix         = '\n'
	OpenCurlyBracket   = '{'
	OpenRoundBracket   = '('
	OpenSquareBracket  = '['
	Semicolon          = ';'
	SingleQuote        = '\''
	Space              = ' '
	Tab                = '\t'
	Underscore         = '_'
)

// Internal prefixes values the optimizer marks as not to be touched.
const Internal = "-clean-css-"

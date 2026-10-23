package writer

import (
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
)

// Styles serializes the optimized tokens into the final output, applying the
// format option (line breaks, indentation, spaces and wrapping).
func Styles(tokens []*token.Token, opts *options.Options) string {
	c := &context{
		format:                 opts.Format,
		line:                   1,
		spaceAfterClosingBrace: opts.Compatibility.Bool("properties", "spaceAfterClosingBrace"),
		wraps:                  opts.Format != nil && opts.Format.Wraps(),
	}
	c.all(tokens)
	return c.String()
}

// oneTime returns the context of a compact serialization used for
// comparisons. It has no format and, like the original, an indentation that
// is still undefined: comments are followed by the text "undefined".
func oneTime() *context {
	return &context{indentWith: "undefined"}
}

// All serializes a list of block-level tokens compactly.
func All(tokens []*token.Token) string {
	c := oneTime()
	c.all(tokens)
	return c.String()
}

// Body serializes the declarations of a rule compactly.
func Body(tokens []*token.Token) string {
	c := oneTime()
	c.body(tokens)
	return c.String()
}

// Property serializes the token at position of a body compactly, followed
// by a semicolon when it is the first one.
func Property(tokens []*token.Token, position int) string {
	c := oneTime()
	c.property(tokens, position, 1, false)
	return c.String()
}

// Rules serializes a list of selectors compactly.
func Rules(tokens []*token.Token) string {
	c := oneTime()
	c.rules(tokens)
	return c.String()
}

// Value serializes the values of a property compactly.
func Value(property *token.Token) string {
	c := oneTime()
	c.value(property)
	return c.String()
}

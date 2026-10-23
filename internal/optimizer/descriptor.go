package optimizer

import (
	"strings"

	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
)

// ValueOptimizer rewrites one value of the named property.
type ValueOptimizer func(name, value string, opts *options.Options) string

// PropertyOptimizer rewrites a whole property of the rule (serialized
// selectors) it belongs to.
type PropertyOptimizer func(rule string, property *Property, opts *options.Options)

// OverrideCheck reports whether value2 can override value1; position is the
// index of the component for shorthands.
type OverrideCheck func(v *Validator, value1, value2 string, position int) bool

// BreakUp splits a shorthand into its components.
type BreakUp func(property *Property, v *Validator) ([]*Property, error)

// Restore puts a shorthand back together from its components.
type Restore func(property *Property) []*token.Token

// InvalidPropertyError reports a shorthand whose values cannot be broken up;
// the property is dropped with the message as a warning.
type InvalidPropertyError struct {
	Message string
}

func (e *InvalidPropertyError) Error() string {
	return e.Message
}

// Descriptor describes how the optimizers treat one CSS property.
type Descriptor struct {
	// BreakUp and Restore convert a shorthand into components and back.
	BreakUp BreakUp
	Restore Restore
	// CanOverride is nil when the property has no override check.
	CanOverride OverrideCheck
	// ComponentOf lists the shorthands the property is a component of and
	// Components the components of a shorthand.
	ComponentOf []string
	Components  []string
	// DefaultValue is the initial value; DefaultIsList marks the values given
	// as a list (one or two values), which compare as their comma-joined text.
	DefaultValue  []string
	DefaultIsList bool
	DoubleValues  bool
	// IntoMultiplexMode is "real", "default" or "placeholder" (empty when unset).
	IntoMultiplexMode string
	KeepUnlessDefault string
	MultiplexLastOnly bool
	// NonMergeableValue is set when HasNonMergeableValue is true.
	NonMergeableValue      string
	HasNonMergeableValue   bool
	OppositeTo             string
	OverridesShorthands    []string
	HasOverridesShorthands bool
	PlaceholderValue       []string
	PropertyOptimizer      PropertyOptimizer
	ShortestValue          string
	Shorthand              bool
	ShorthandComponents    bool
	SingleTypeComponents   bool
	// ValueOptimizers is nil when the property does not list any; the
	// whitespace optimizer is used then.
	ValueOptimizers []ValueOptimizer
}

// Default returns the default value as the text it compares as.
func (d *Descriptor) Default() string {
	return strings.Join(d.DefaultValue, ",")
}

// DefaultToken returns a new value token holding the default value.
func (d *Descriptor) DefaultToken() *token.Token {
	return token.NewValue(d.Default())
}

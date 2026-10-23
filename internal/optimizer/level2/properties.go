package level2

import (
	"errors"
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
)

// optimizeProperties optimizes the declarations of one body: shorthands are
// broken into components, longhands merged into shorthands and overridden
// declarations dropped, then the tokens are rewritten in place.
func optimizeProperties(properties *[]*token.Token, withOverriding, withMerging bool, ctx *optimizer.Context) {
	opts := levelOptions(ctx)
	wrapped := optimizer.WrapAll(properties, skippedProperties(opts.Get("skipProperties")))

	populateComponents(wrapped, ctx.Validator, &ctx.Warnings)

	for _, property := range wrapped {
		if property.Block {
			optimizeProperties(&property.Value[0].Body, withOverriding, withMerging, ctx)
		}
	}

	if withMerging && opts.Bool("mergeIntoShorthands") {
		mergeIntoShorthands(&wrapped, ctx.Validator)
	}

	if withOverriding && opts.Bool("overrideProperties") {
		overrideProperties(wrapped, withMerging, ctx)
	}

	optimizer.RestoreFromOptimizing(wrapped, restoreWithComponents)
	optimizer.RemoveUnused(wrapped)
}

func skippedProperties(value any) []string {
	list, _ := value.([]any)
	skipped := []string{}
	for _, item := range list {
		if name, ok := item.(string); ok {
			skipped = append(skipped, name)
		}
	}
	return skipped
}

// restoreWithComponents rebuilds the value of a shorthand from its components.
func restoreWithComponents(property *optimizer.Property) []*token.Token {
	if d := lookup(property.Name); d != nil && d.Shorthand {
		return d.Restore(property)
	}
	return property.Value
}

// populateComponents breaks every shorthand into its components; a shorthand
// that cannot be broken up is marked unused with a warning.
func populateComponents(properties []*optimizer.Property, validator *optimizer.Validator, warnings *[]string) {
	for i := len(properties) - 1; i >= 0; i-- {
		property := properties[i]
		descriptor := lookup(property.Name)

		if property.Dynamic || descriptor == nil || !descriptor.Shorthand {
			continue
		}

		if onlyValueIsVariable(property, validator) || moreThanOneValueIsVariable(property, validator) {
			property.Optimizable = false
			continue
		}

		property.Shorthand = true
		property.Dirty = true

		if err := breakUp(property, descriptor, validator); err != nil {
			property.Components = []*optimizer.Property{}
			var invalid *optimizer.InvalidPropertyError
			if errors.As(err, &invalid) {
				*warnings = append(*warnings, invalid.Message)
			}
		}

		if len(property.Components) > 0 {
			property.Multiplex = property.Components[0].Multiplex
		} else {
			property.Unused = true
		}
	}
}

func breakUp(property *optimizer.Property, descriptor *optimizer.Descriptor, validator *optimizer.Validator) error {
	components, err := descriptor.BreakUp(property, validator)
	if err != nil {
		return err
	}
	property.Components = components

	if descriptor.ShorthandComponents {
		for _, component := range components {
			componentDescriptor := lookup(component.Name)
			sub, err := componentDescriptor.BreakUp(component, validator)
			if err != nil {
				return err
			}
			component.Components = sub
		}
	}
	return nil
}

func onlyValueIsVariable(property *optimizer.Property, validator *optimizer.Validator) bool {
	return len(property.Value) == 1 && validator.IsVariable(property.Value[0].Value)
}

func moreThanOneValueIsVariable(property *optimizer.Property, validator *optimizer.Validator) bool {
	if len(property.Value) < 2 {
		return false
	}
	count := 0
	for _, value := range property.Value {
		if validator.IsVariable(value.Value) {
			count++
		}
	}
	return count > 1
}

// pairCheck compares one value of each property; position is the value index.
type pairCheck func(left, right string, position int) bool

// overrideCheckFor returns the override check of a property bound to the
// validator: its configured check, or plain equality.
func overrideCheckFor(name string, validator *optimizer.Validator) pairCheck {
	if d := lookup(name); d != nil && d.CanOverride != nil {
		check := d.CanOverride
		return func(left, right string, position int) bool {
			return check(validator, left, right, position)
		}
	}
	return sameValue
}

func sameValue(left, right string, _ int) bool {
	return left == right
}

// everyValuesPair applies check to each pair of values at the same position,
// skipping commas; a missing or empty value repeats the previous one.
func everyValuesPair(check pairCheck, left, right *optimizer.Property) bool {
	if left == nil || right == nil {
		return false
	}
	total := max(len(left.Value), len(right.Value))
	var leftValue, rightValue string

	for position := 0; position < total; position++ {
		if text := valueText(left, position); text != "" {
			leftValue = text
		}
		if text := valueText(right, position); text != "" {
			rightValue = text
		}

		if leftValue == "," || rightValue == "," {
			continue
		}

		if !check(leftValue, rightValue, position) {
			return false
		}
	}

	return true
}

// findComponentIn finds the component of shorthand with the name of
// longhand, looking one level deeper for shorthands of shorthands.
func findComponentIn(shorthand, longhand *optimizer.Property) *optimizer.Property {
	if found := findInDirectComponents(shorthand, longhand.Name); found != nil {
		return found
	}

	if d := lookup(shorthand.Name); d == nil || !d.ShorthandComponents {
		return nil
	}
	for _, component := range shorthand.Components {
		if found := findInDirectComponents(component, longhand.Name); found != nil {
			return found
		}
	}
	return nil
}

func findInDirectComponents(shorthand *optimizer.Property, name string) *optimizer.Property {
	for _, component := range shorthand.Components {
		if component.Name == name {
			return component
		}
	}
	return nil
}

func hasInherit(property *optimizer.Property) bool {
	return hasValue(property, "inherit")
}

func hasUnset(property *optimizer.Property) bool {
	return hasValue(property, "unset")
}

func hasValue(property *optimizer.Property, value string) bool {
	for i := len(property.Value) - 1; i >= 0; i-- {
		if property.Value[i].Value == value {
			return true
		}
	}
	return false
}

func hasSameValues(property *optimizer.Property) bool {
	firstValue := valueText(property, 0)
	for i := 1; i < len(property.Value); i++ {
		if property.Value[i].Value != firstValue {
			return false
		}
	}
	return true
}

// isComponentOf reports whether property2 is a component of property1, or
// unless shallow, a component of one of its components.
func isComponentOf(property1, property2 *optimizer.Property, shallow bool) bool {
	if isDirectComponentOf(property1, property2) {
		return true
	}
	if shallow {
		return false
	}
	if d := lookup(property1.Name); d == nil || !d.ShorthandComponents {
		return false
	}
	for _, component := range property1.Components {
		if isDirectComponentOf(component, property2) {
			return true
		}
	}
	return false
}

func isDirectComponentOf(property1, property2 *optimizer.Property) bool {
	d := lookup(property1.Name)
	return d != nil && d.Components != nil && slices.Contains(d.Components, property2.Name)
}

// isMergeableShorthand reports whether a shorthand can take in longhands: a
// font given as a single keyword cannot.
func isMergeableShorthand(shorthand *optimizer.Property) bool {
	if shorthand.Name != "font" {
		return true
	}
	return !strings.Contains(valueText(shorthand, 0), token.Internal)
}

func overridesNonComponentShorthand(property1, property2 *optimizer.Property) bool {
	d := lookup(property1.Name)
	return d != nil && d.HasOverridesShorthands && slices.Contains(d.OverridesShorthands, property2.Name)
}

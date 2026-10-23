package level2

import (
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

func wouldBreakCompatibility(property *optimizer.Property, validator *optimizer.Validator) bool {
	for _, component := range property.Components {
		descriptor := lookup(component.Name)
		check := overrideCheckFor(component.Name, validator)

		defaultComponent := optimizer.ShallowClone(component)
		defaultComponent.Value = []*token.Token{descriptor.DefaultToken()}

		if !everyValuesPair(check, defaultComponent, component) {
			return true
		}
	}
	return false
}

func overrideIntoMultiplex(property, by *optimizer.Property) {
	by.Unused = true
	turnIntoMultiplex(by, multiplexSize(property))
	property.Value = by.Value
}

func overrideByMultiplex(property, by *optimizer.Property) {
	by.Unused = true
	property.Multiplex = true
	property.Value = by.Value
}

func overrideSimple(property, by *optimizer.Property) {
	by.Unused = true
	property.Value = by.Value
}

func override(property, by *optimizer.Property) {
	switch {
	case by.Multiplex:
		overrideByMultiplex(property, by)
	case property.Multiplex:
		overrideIntoMultiplex(property, by)
	default:
		overrideSimple(property, by)
	}
}

func overrideShorthand(property, by *optimizer.Property) {
	by.Unused = true
	for i := range property.Components {
		override(property.Components[i], by.Components[i])
	}
}

func turnIntoMultiplex(property *optimizer.Property, size int) {
	property.Multiplex = true
	if d := lookup(property.Name); d != nil && d.Shorthand {
		for _, component := range property.Components {
			if !component.Multiplex {
				turnLonghandValueIntoMultiplex(component, size)
			}
		}
	} else {
		turnLonghandValueIntoMultiplex(property, size)
	}
}

func turnLonghandValueIntoMultiplex(property *optimizer.Property, size int) {
	descriptor := lookup(property.Name)
	withRealValue := descriptor.IntoMultiplexMode == "real"

	var fill func() []*token.Token
	switch {
	case withRealValue:
		values := append([]*token.Token{}, property.Value...)
		fill = func() []*token.Token { return values }
	case descriptor.IntoMultiplexMode == "placeholder":
		fill = func() []*token.Token { return valueTokens(descriptor.PlaceholderValue) }
	case descriptor.DefaultIsList:
		fill = func() []*token.Token { return valueTokens(descriptor.DefaultValue) }
	default:
		fill = func() []*token.Token { return []*token.Token{descriptor.DefaultToken()} }
	}

	for i := multiplexSize(property); i < size; i++ {
		property.Value = append(property.Value, token.NewValue(","))
		property.Value = append(property.Value, fill()...)
	}
}

func valueTokens(values []string) []*token.Token {
	tokens := make([]*token.Token, len(values))
	for i, value := range values {
		tokens[i] = token.NewValue(value)
	}
	return tokens
}

func multiplexSize(component *optimizer.Property) int {
	size := 0
	for _, value := range component.Value {
		if value.Value == "," {
			size++
		}
	}
	return size + 1
}

// lengthOf returns the length of a declaration of the property with its
// current values, as written by the compact serializer.
func lengthOf(property *optimizer.Property) int {
	fake := token.NewProperty(property.Name, property.Value...)
	return jsutil.Len16(writer.Property([]*token.Token{fake}, 0))
}

func moreSameShorthands(properties []*optimizer.Property, startAt int, name string) bool {
	count := 0
	for i := startAt; i >= 0; i-- {
		if properties[i].Name == name && !properties[i].Unused {
			count++
		}
		if count > 1 {
			break
		}
	}
	return count > 1
}

func overridingFunction(shorthand *optimizer.Property, validator *optimizer.Validator) bool {
	for _, component := range shorthand.Components {
		if !anyValue(validator.IsUrl, component) && anyValue(validator.IsFunction, component) {
			return true
		}
	}
	return false
}

func anyValue(fn func(string) bool, property *optimizer.Property) bool {
	for _, value := range property.Value {
		if value.Value == "," {
			continue
		}
		if fn(value.Value) {
			return true
		}
	}
	return false
}

func wouldResultInLongerValue(left, right *optimizer.Property) bool {
	if !left.Multiplex && !right.Multiplex || left.Multiplex && right.Multiplex {
		return false
	}

	multiplex, simple := right, left
	if left.Multiplex {
		multiplex, simple = left, right
	}

	multiplexClone := optimizer.DeepClone(multiplex)
	optimizer.RestoreFromOptimizing([]*optimizer.Property{multiplexClone}, restoreWithComponents)

	simpleClone := optimizer.DeepClone(simple)
	optimizer.RestoreFromOptimizing([]*optimizer.Property{simpleClone}, restoreWithComponents)

	lengthBefore := lengthOf(multiplexClone) + 1 + lengthOf(simpleClone)

	if left.Multiplex {
		component := findComponentIn(multiplexClone, simpleClone)
		overrideIntoMultiplex(component, simpleClone)
	} else {
		component := findComponentIn(simpleClone, multiplexClone)
		turnIntoMultiplex(simpleClone, multiplexSize(multiplexClone))
		overrideByMultiplex(component, multiplexClone)
	}

	optimizer.RestoreFromOptimizing([]*optimizer.Property{simpleClone}, restoreWithComponents)

	return lengthBefore <= lengthOf(simpleClone)
}

func isCompactable(property *optimizer.Property) bool {
	return lookup(property.Name) != nil
}

func noneOverrideHack(left, right *optimizer.Property) bool {
	return !left.Multiplex &&
		(left.Name == "background" || left.Name == "background-image") &&
		right.Multiplex &&
		(right.Name == "background" || right.Name == "background-image") &&
		anyLayerIsNone(right.Value)
}

func anyLayerIsNone(values []*token.Token) bool {
	for _, layer := range intoLayers(values) {
		if len(layer) == 1 && layer[0].Value == "none" {
			return true
		}
	}
	return false
}

func intoLayers(values []*token.Token) [][]*token.Token {
	var layers [][]*token.Token
	var layer []*token.Token
	for _, value := range values {
		if value.Value == "," {
			layers = append(layers, layer)
			layer = nil
		} else {
			layer = append(layer, value)
		}
	}
	return append(layers, layer)
}

// overrideProperties drops declarations that later ones override and pulls
// longhands into earlier shorthands where that is safe.
func overrideProperties(properties []*optimizer.Property, withMerging bool, ctx *optimizer.Context) {
	compatibility := ctx.Options.Compatibility
	validator := ctx.Validator
	var mayOverride pairCheck

propertyLoop:
	for i := len(properties) - 1; i >= 0; i-- {
		right := properties[i]

		if !isCompactable(right) || right.Block {
			continue
		}

		mayOverride = overrideCheckFor(right.Name, validator)

	traverseLoop:
		for j := i - 1; j >= 0; j-- {
			left := properties[j]

			if !isCompactable(left) || left.Block {
				continue
			}
			if left.Dynamic || right.Dynamic {
				continue
			}
			if left.Unused || right.Unused {
				continue
			}
			if left.Hack != nil && right.Hack == nil && !right.Important || left.Hack == nil && !left.Important && right.Hack != nil {
				continue
			}
			if left.Important == right.Important && left.HackKind() != right.HackKind() {
				continue
			}
			if left.Important == right.Important &&
				(left.HackKind() != right.HackKind() || left.HackSuffix() != "" && left.HackSuffix() != right.HackSuffix()) {
				continue
			}
			if hasInherit(right) {
				continue
			}
			if noneOverrideHack(left, right) {
				continue
			}

			switch {
			case right.Shorthand && isComponentOf(right, left, false):
				// maybe `left` can be overridden by `right` which is a shorthand?
				if !right.Important && left.Important {
					continue
				}
				if !anyValue(validator.IsFunction, left) && overridingFunction(right, validator) {
					continue
				}
				if !isMergeableShorthand(right) {
					left.Unused = true
					continue
				}

				component := findComponentIn(right, left)
				mayOverride = overrideCheckFor(left.Name, validator)
				if everyValuesPair(mayOverride, left, component) {
					left.Unused = true
				}

			case right.Shorthand && overridesNonComponentShorthand(right, left):
				// `right` is a shorthand while `left` can be overriden by it, think `border` and `border-top`
				if !right.Important && left.Important {
					continue
				}
				if !anyValue(validator.IsFunction, left) && overridingFunction(right, validator) {
					continue
				}

				overriddenComponents := []*optimizer.Property{left}
				if left.Shorthand {
					overriddenComponents = left.Components
				}

				for k := len(overriddenComponents) - 1; k >= 0; k-- {
					overriddenComponent := overriddenComponents[k]
					overridingComponent := findComponentIn(right, overriddenComponent)
					mayOverride = overrideCheckFor(overriddenComponent.Name, validator)

					if !everyValuesPair(mayOverride, left, overridingComponent) {
						continue traverseLoop
					}
				}

				left.Unused = true

			case withMerging && left.Shorthand && !right.Shorthand && isComponentOf(left, right, true):
				// maybe `right` can be pulled into `left` which is a shorthand?
				if right.Important && !left.Important {
					continue
				}
				if !right.Important && left.Important {
					right.Unused = true
					continue
				}

				// Pending more clever algorithm in #527
				if moreSameShorthands(properties, i-1, left.Name) {
					continue
				}
				if overridingFunction(left, validator) {
					continue
				}
				if !isMergeableShorthand(left) {
					continue
				}
				if hasUnset(left) || hasUnset(right) {
					continue
				}

				component := findComponentIn(left, right)
				if !everyValuesPair(mayOverride, component, right) {
					continue
				}

				disabledBackgroundMerging := !compatibility.Bool("properties", "backgroundClipMerging") && strings.Contains(component.Name, "background-clip") ||
					!compatibility.Bool("properties", "backgroundOriginMerging") && strings.Contains(component.Name, "background-origin") ||
					!compatibility.Bool("properties", "backgroundSizeMerging") && strings.Contains(component.Name, "background-size")
				rightDescriptor := lookup(right.Name)
				nonMergeableValue := rightDescriptor.HasNonMergeableValue && len(right.Value) > 0 &&
					rightDescriptor.NonMergeableValue == right.Value[0].Value

				if disabledBackgroundMerging || nonMergeableValue {
					continue
				}
				if !compatibility.Bool("properties", "merging") && wouldBreakCompatibility(left, validator) {
					continue
				}
				if valueText(component, 0) != valueText(right, 0) && (hasInherit(left) || hasInherit(right)) {
					continue
				}
				if wouldResultInLongerValue(left, right) {
					continue
				}

				if !left.Multiplex && right.Multiplex {
					turnIntoMultiplex(left, multiplexSize(right))
				}

				override(component, right)
				left.Dirty = true

			case withMerging && left.Shorthand && right.Shorthand && left.Name == right.Name:
				// merge if all components can be merged
				if !left.Multiplex && right.Multiplex {
					continue
				}
				if !right.Important && left.Important {
					right.Unused = true
					continue propertyLoop
				}
				if right.Important && !left.Important {
					left.Unused = true
					continue
				}
				if !isMergeableShorthand(right) {
					left.Unused = true
					continue
				}

				for k := len(left.Components) - 1; k >= 0; k-- {
					leftComponent := left.Components[k]
					var rightComponent *optimizer.Property
					if k < len(right.Components) {
						rightComponent = right.Components[k]
					}

					mayOverride = overrideCheckFor(leftComponent.Name, validator)
					if !everyValuesPair(mayOverride, leftComponent, rightComponent) {
						continue propertyLoop
					}
				}

				overrideShorthand(left, right)
				left.Dirty = true

			case withMerging && left.Shorthand && right.Shorthand && isComponentOf(left, right, false):
				// border is a shorthand but any of its components is a shorthand too
				if !left.Important && right.Important {
					continue
				}

				component := findComponentIn(left, right)
				mayOverride = overrideCheckFor(right.Name, validator)
				if !everyValuesPair(mayOverride, component, right) {
					continue
				}

				if left.Important && !right.Important {
					right.Unused = true
					continue
				}

				if len(lookup(right.Name).Restore(right)) > 1 {
					continue
				}

				component = findComponentIn(left, right)
				override(component, right)
				right.Dirty = true

			case left.Name == right.Name:
				// two non-shorthands should be merged based on understandability
				overridable := true

				if right.Shorthand {
					for k := len(right.Components) - 1; k >= 0 && overridable; k-- {
						var overriddenComponent *optimizer.Property
						if k < len(left.Components) {
							overriddenComponent = left.Components[k]
						}
						overridingComponent := right.Components[k]
						mayOverride = overrideCheckFor(overridingComponent.Name, validator)

						overridable = everyValuesPair(mayOverride, overriddenComponent, overridingComponent)
					}
				} else {
					mayOverride = overrideCheckFor(right.Name, validator)
					overridable = everyValuesPair(mayOverride, left, right)
				}

				if left.Important && !right.Important && overridable {
					right.Unused = true
					continue
				}
				if !left.Important && right.Important && overridable {
					left.Unused = true
					continue
				}
				if !overridable {
					continue
				}

				left.Unused = true
			}
		}
	}
}

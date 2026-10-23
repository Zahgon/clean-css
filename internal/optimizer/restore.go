package optimizer

import (
	"strings"

	"github.com/clean-css/clean-css/internal/token"
)

// layerRestore restores one layer of a possibly multiplexed shorthand;
// lastInMultiplex is false for every layer but the last one.
type layerRestore func(property *Property, lastInMultiplex bool) []*token.Token

func isInheritOnly(values []*token.Token) bool {
	for _, value := range values {
		if value.Value != "inherit" && value.Value != "," && value.Value != "/" {
			return false
		}
	}
	return true
}

func isDefaultComponent(component *Property) bool {
	descriptor := configuration[component.Name]
	value := func(i int) string {
		if i < len(component.Value) {
			return component.Value[i].Value
		}
		return ""
	}

	switch {
	case descriptor.DoubleValues && len(descriptor.DefaultValue) == 1:
		return value(0) == descriptor.DefaultValue[0] &&
			(len(component.Value) < 2 || value(1) == descriptor.DefaultValue[0])
	case descriptor.DoubleValues && len(descriptor.DefaultValue) != 1:
		second := value(0)
		if len(component.Value) > 1 {
			second = value(1)
		}
		return value(0) == descriptor.DefaultValue[0] && second == descriptor.DefaultValue[1]
	}
	return value(0) == descriptor.Default()
}

func restoreBackgroundLayer(property *Property, lastInMultiplex bool) []*token.Token {
	components := property.Components
	var restored []*token.Token

	restoreValue := func(component *Property) {
		restored = append(token.Copy(component.Value), restored...)
	}

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]
		isDefault := isDefaultComponent(component)

		switch component.Name {
		case "background-clip":
			originComponent := components[i-1]
			isOriginDefault := isDefaultComponent(originComponent)

			needsOne := component.Value[0].Value == originComponent.Value[0].Value
			needsBoth := !needsOne && (isOriginDefault && !isDefault ||
				!isOriginDefault && !isDefault ||
				!isOriginDefault && isDefault && component.Value[0].Value != originComponent.Value[0].Value)

			if needsOne {
				restoreValue(originComponent)
			} else if needsBoth {
				restoreValue(component)
				restoreValue(originComponent)
			}
			i--
		case "background-size":
			positionComponent := components[i-1]
			isPositionDefault := isDefaultComponent(positionComponent)

			needsOne := !isPositionDefault && isDefault
			needsBoth := !needsOne && (isPositionDefault && !isDefault || !isPositionDefault && !isDefault)

			switch {
			case needsOne:
				restoreValue(positionComponent)
			case needsBoth:
				restoreValue(component)
				restored = append([]*token.Token{token.NewValue("/")}, restored...)
				restoreValue(positionComponent)
			case len(positionComponent.Value) == 1:
				restoreValue(positionComponent)
			}
			i--
		default:
			if isDefault || configuration[component.Name].MultiplexLastOnly && !lastInMultiplex {
				continue
			}
			restoreValue(component)
		}
	}

	if len(restored) == 0 && len(property.Value) == 1 && property.Value[0].Value == "0" {
		restored = append(restored, property.Value[0])
	}

	if len(restored) == 0 {
		restored = append(restored, configuration[property.Name].DefaultToken())
	}

	if isInheritOnly(restored) {
		return []*token.Token{restored[0]}
	}

	return restored
}

func restoreBorderRadius(property *Property) []*token.Token {
	if !property.Multiplex {
		return restoreFourValues(property)
	}

	horizontal := ShallowClone(property)
	vertical := ShallowClone(property)

	for i := 0; i < 4; i++ {
		component := property.Components[i]

		horizontalComponent := ShallowClone(property)
		horizontalComponent.Value = []*token.Token{component.Value[0]}
		horizontal.Components = append(horizontal.Components, horizontalComponent)

		verticalComponent := ShallowClone(property)
		// only the shorthand break-up knows that border radius longhands have
		// two values, the tokenizer does not add a missing second one
		if len(component.Value) > 1 {
			verticalComponent.Value = []*token.Token{component.Value[1]}
		} else {
			verticalComponent.Value = []*token.Token{component.Value[0]}
		}
		vertical.Components = append(vertical.Components, verticalComponent)
	}

	horizontalValues := restoreFourValues(horizontal)
	verticalValues := restoreFourValues(vertical)

	same := len(horizontalValues) == len(verticalValues)
	for i := 0; same && i < len(horizontalValues) && i < 4; i++ {
		same = horizontalValues[i].Value == verticalValues[i].Value
	}
	if same {
		return horizontalValues
	}

	restored := append(token.Copy(horizontalValues), token.NewValue("/"))
	return append(restored, verticalValues...)
}

func restoreFont(property *Property) []*token.Token {
	components := property.Components
	var restored []*token.Token
	componentIndex := 0

	if strings.HasPrefix(property.Value[0].Value, token.Internal) {
		property.Value[0].Value = property.Value[0].Value[len(token.Internal):]
		return property.Value
	}

	// first four components are optional
	for ; componentIndex < 4; componentIndex++ {
		component := components[componentIndex]
		if component.Value[0].Value != configuration[component.Name].Default() {
			restored = append(restored, component.Value...)
		}
	}

	// then comes font-size
	restored = append(restored, components[componentIndex].Value...)
	componentIndex++

	// then may come line-height
	if components[componentIndex].Value[0].Value != configuration[components[componentIndex].Name].Default() {
		restored = append(restored, token.NewValue("/"))
		restored = append(restored, components[componentIndex].Value...)
	}
	componentIndex++

	// then comes font-family
	family := components[componentIndex].Value
	for i, value := range family {
		restored = append(restored, value)
		if i+1 < len(family) {
			restored = append(restored, token.NewValue(","))
		}
	}

	if isInheritOnly(restored) {
		return []*token.Token{restored[0]}
	}

	return restored
}

func restoreFourValues(property *Property) []*token.Token {
	components := property.Components
	value1 := components[0].Value[0]
	value2 := components[1].Value[0]
	value3 := components[2].Value[0]
	value4 := components[3].Value[0]

	switch {
	case value1.Value == value2.Value && value1.Value == value3.Value && value1.Value == value4.Value:
		return []*token.Token{value1}
	case value1.Value == value3.Value && value2.Value == value4.Value:
		return []*token.Token{value1, value2}
	case value2.Value == value4.Value:
		return []*token.Token{value1, value2, value3}
	}
	return []*token.Token{value1, value2, value3, value4}
}

// restoreMultiplex restores a shorthand layer by layer with restoreWith.
func restoreMultiplex(restoreWith layerRestore) Restore {
	return func(property *Property) []*token.Token {
		if !property.Multiplex {
			return restoreWith(property, true)
		}

		multiplexSize := 0
		var restored []*token.Token
		componentMultiplexSoFar := map[string]int{}

		// At this point we don't know what's the multiplex size, e.g. how many background layers are there
		for _, value := range property.Components[0].Value {
			if value.Value == "," {
				multiplexSize++
			}
		}

		for i := 0; i <= multiplexSize; i++ {
			layer := ShallowClone(property)

			// We split multiplex into parts and restore them one by one
			for _, componentToClone := range property.Components {
				component := ShallowClone(componentToClone)
				layer.Components = append(layer.Components, component)

				// The trick is some properties has more than one value, so we iterate over values looking for
				// a multiplex separator - a comma
				for k := componentMultiplexSoFar[component.Name]; k < len(componentToClone.Value); k++ {
					if componentToClone.Value[k].Value == "," {
						componentMultiplexSoFar[component.Name] = k + 1
						break
					}
					component.Value = append(component.Value, componentToClone.Value[k])
				}
			}

			// No we can restore shorthand value
			restored = append(restored, restoreWith(layer, i == multiplexSize)...)

			if i < multiplexSize {
				restored = append(restored, token.NewValue(","))
			}
		}

		return restored
	}
}

func restoreWithoutDefaultsLayer(property *Property, _ bool) []*token.Token {
	components := property.Components
	var restored []*token.Token

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]
		descriptor := configuration[component.Name]

		if component.Value[0].Value != descriptor.Default() ||
			descriptor.KeepUnlessDefault != "" && !isDefault(components, descriptor.KeepUnlessDefault) {
			restored = append([]*token.Token{component.Value[0]}, restored...)
		}
	}

	if len(restored) == 0 {
		restored = append(restored, configuration[property.Name].DefaultToken())
	}

	if isInheritOnly(restored) {
		return []*token.Token{restored[0]}
	}

	return restored
}

func restoreWithoutDefaults(property *Property) []*token.Token {
	return restoreWithoutDefaultsLayer(property, true)
}

func isDefault(components []*Property, propertyName string) bool {
	for _, component := range components {
		if component.Name == propertyName && component.Value[0].Value == configuration[propertyName].Default() {
			return true
		}
	}
	return false
}

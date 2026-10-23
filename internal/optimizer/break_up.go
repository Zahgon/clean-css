package optimizer

import (
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/token"
)

func invalid(message string) error {
	return &InvalidPropertyError{Message: message}
}

// positionOf formats the first position of a token for a warning.
func positionOf(t *token.Token) string {
	if t == nil || len(t.Positions) == 0 {
		return "undefined"
	}
	return t.Positions[0].String()
}

// namePositionOf formats the position of the name of the property token a
// wrapped property comes from.
func namePositionOf(property *Property) string {
	if property.All == nil {
		return "undefined"
	}
	return positionOf(property.Token().Name)
}

func anyIsInherit(values []*token.Token) bool {
	for _, value := range values {
		if value.Value == "inherit" {
			return true
		}
	}
	return false
}

func colorFilter(v *Validator) func(*token.Token) bool {
	return func(value *token.Token) bool {
		return value.Value == "invert" || v.IsColor(value.Value) || v.IsPrefixed(value.Value)
	}
}

func styleFilter(v *Validator) func(*token.Token) bool {
	return func(value *token.Token) bool {
		return value.Value != "inherit" && v.IsStyleKeyword(value.Value) && !v.IsColorFunction(value.Value)
	}
}

func widthFilter(v *Validator) func(*token.Token) bool {
	return func(value *token.Token) bool {
		return value.Value != "inherit" &&
			(v.IsWidth(value.Value) || v.IsUnit(value.Value) || v.IsDynamicUnit(value.Value)) &&
			!v.IsStyleKeyword(value.Value) &&
			!v.IsColorFunction(value.Value)
	}
}

func filterValues(values []*token.Token, keep func(*token.Token) bool) []*token.Token {
	var kept []*token.Token
	for _, value := range values {
		if keep(value) {
			kept = append(kept, value)
		}
	}
	return kept
}

// WrapDefault returns a property named name holding its default value.
func WrapDefault(name string) *Property {
	descriptor := configuration[name]
	switch {
	case descriptor.DoubleValues && len(descriptor.DefaultValue) == 2:
		return WrapSingle(token.NewProperty(name, token.NewValue(descriptor.DefaultValue[0]), token.NewValue(descriptor.DefaultValue[1])))
	case descriptor.DoubleValues && len(descriptor.DefaultValue) == 1:
		return WrapSingle(token.NewProperty(name, token.NewValue(descriptor.DefaultValue[0])))
	}
	return WrapSingle(token.NewProperty(name, descriptor.DefaultToken()))
}

func breakUpAnimation(property *Property, v *Validator) ([]*Property, error) {
	duration := WrapDefault(property.Name + "-duration")
	timing := WrapDefault(property.Name + "-timing-function")
	delay := WrapDefault(property.Name + "-delay")
	iteration := WrapDefault(property.Name + "-iteration-count")
	direction := WrapDefault(property.Name + "-direction")
	fill := WrapDefault(property.Name + "-fill-mode")
	play := WrapDefault(property.Name + "-play-state")
	name := WrapDefault(property.Name + "-name")
	components := []*Property{duration, timing, delay, iteration, direction, fill, play, name}
	values := property.Value
	var durationSet, timingSet, delaySet, iterationSet, directionSet, fillSet, playSet, nameSet bool

	if len(values) == 1 && values[0].Value == "inherit" {
		for _, component := range components {
			component.Value = values
		}
		return components, nil
	}

	if len(values) > 1 && anyIsInherit(values) {
		return nil, invalid("Invalid animation values at " + positionOf(values[0]) + ". Ignoring.")
	}

	for _, value := range values {
		text := value.Value
		switch {
		case v.IsTime(text) && !durationSet:
			duration.Value = []*token.Token{value}
			durationSet = true
		case v.IsTime(text) && !delaySet:
			delay.Value = []*token.Token{value}
			delaySet = true
		case (v.IsGlobal(text) || v.IsTimingFunction(text)) && !timingSet:
			timing.Value = []*token.Token{value}
			timingSet = true
		case (v.IsAnimationIterationCountKeyword(text) || v.IsPositiveNumber(text)) && !iterationSet:
			iteration.Value = []*token.Token{value}
			iterationSet = true
		case v.IsAnimationDirectionKeyword(text) && !directionSet:
			direction.Value = []*token.Token{value}
			directionSet = true
		case v.IsAnimationFillModeKeyword(text) && !fillSet:
			fill.Value = []*token.Token{value}
			fillSet = true
		case v.IsAnimationPlayStateKeyword(text) && !playSet:
			play.Value = []*token.Token{value}
			playSet = true
		case (v.IsAnimationNameKeyword(text) || v.IsIdentifier(text)) && !nameSet:
			name.Value = []*token.Token{value}
			nameSet = true
		default:
			return nil, invalid("Invalid animation value at " + positionOf(value) + ". Ignoring.")
		}
	}

	return components, nil
}

func breakUpBackground(property *Property, v *Validator) ([]*Property, error) {
	image := WrapDefault("background-image")
	position := WrapDefault("background-position")
	size := WrapDefault("background-size")
	repeat := WrapDefault("background-repeat")
	attachment := WrapDefault("background-attachment")
	origin := WrapDefault("background-origin")
	clip := WrapDefault("background-clip")
	color := WrapDefault("background-color")
	components := []*Property{image, position, size, repeat, attachment, origin, clip, color}
	values := property.Value

	var positionSet, clipSet, originSet, repeatSet, anyValueSet bool

	if len(values) == 1 && values[0].Value == "inherit" {
		// NOTE: 'inherit' is not a valid value for background-attachment
		for _, component := range []*Property{color, image, repeat, position, size, origin, clip} {
			component.Value = values
		}
		return components, nil
	}

	if len(values) == 1 && values[0].Value == "0 0" {
		return components, nil
	}

	for i := len(values) - 1; i >= 0; i-- {
		value := values[i]
		text := value.Value

		switch {
		case v.IsBackgroundAttachmentKeyword(text):
			attachment.Value = []*token.Token{value}
			anyValueSet = true
		case v.IsBackgroundClipKeyword(text) || v.IsBackgroundOriginKeyword(text):
			if clipSet {
				origin.Value = []*token.Token{value}
				originSet = true
			} else {
				clip.Value = []*token.Token{value}
				clipSet = true
			}
			anyValueSet = true
		case v.IsBackgroundRepeatKeyword(text):
			if repeatSet {
				repeat.Value = append([]*token.Token{value}, repeat.Value...)
			} else {
				repeat.Value = []*token.Token{value}
				repeatSet = true
			}
			anyValueSet = true
		case v.IsBackgroundPositionKeyword(text) || v.IsBackgroundSizeKeyword(text) || v.IsUnit(text) || v.IsDynamicUnit(text):
			if i > 0 {
				previous := values[i-1]
				switch {
				case previous.Value == "/":
					size.Value = []*token.Token{value}
				case i > 1 && values[i-2].Value == "/":
					size.Value = []*token.Token{previous, value}
					i -= 2
				default:
					if !positionSet {
						position.Value = nil
					}
					position.Value = append([]*token.Token{value}, position.Value...)
					positionSet = true
				}
			} else {
				if !positionSet {
					position.Value = nil
				}
				position.Value = append([]*token.Token{value}, position.Value...)
				positionSet = true
			}
			anyValueSet = true
		case (color.Value[0].Value == configuration[color.Name].Default() || color.Value[0].Value == "none") &&
			(v.IsColor(text) || v.IsPrefixed(text)):
			color.Value = []*token.Token{value}
			anyValueSet = true
		case v.IsUrl(text) || v.IsFunction(text):
			image.Value = []*token.Token{value}
			anyValueSet = true
		}
	}

	if clipSet && !originSet {
		origin.Value = token.Copy(clip.Value)
	}

	if !anyValueSet {
		return nil, invalid("Invalid background value at " + positionOf(values[0]) + ". Ignoring.")
	}

	return components, nil
}

func breakUpBorderRadius(property *Property, v *Validator) ([]*Property, error) {
	values := property.Value
	splitAt := -1
	for i, value := range values {
		if value.Value == "/" {
			splitAt = i
			break
		}
	}

	if splitAt == 0 || splitAt == len(values)-1 {
		return nil, invalid("Invalid border-radius value at " + positionOf(values[0]) + ". Ignoring.")
	}

	target := WrapDefault(property.Name)
	if splitAt > -1 {
		target.Value = token.Copy(values[:splitAt])
	} else {
		target.Value = token.Copy(values)
	}
	target.Components = fourValues(target)

	remainder := WrapDefault(property.Name)
	if splitAt > -1 {
		remainder.Value = token.Copy(values[splitAt+1:])
	} else {
		remainder.Value = token.Copy(values)
	}
	remainder.Components = fourValues(remainder)

	for j := 0; j < 4; j++ {
		target.Components[j].Multiplex = true
		target.Components[j].Value = append(token.Copy(target.Components[j].Value), remainder.Components[j].Value...)
	}

	return target.Components, nil
}

func breakUpFont(property *Property, v *Validator) ([]*Property, error) {
	style := WrapDefault("font-style")
	variant := WrapDefault("font-variant")
	weight := WrapDefault("font-weight")
	stretch := WrapDefault("font-stretch")
	size := WrapDefault("font-size")
	height := WrapDefault("line-height")
	family := WrapDefault("font-family")
	components := []*Property{style, variant, weight, stretch, size, height, family}
	values := property.Value
	const fuzzyMatched = 4 // style, variant, weight, and stretch
	index := 0
	var isStretchSet, isStyleSet, isVariantSet, isWeightSet, appendableFamilyName bool

	at := func(i int) *token.Token {
		if i < len(values) {
			return values[i]
		}
		return nil
	}

	if at(index) == nil {
		return nil, invalid("Missing font values at " + namePositionOf(property) + ". Ignoring.")
	}

	if len(values) == 1 && values[0].Value == "inherit" {
		for _, component := range components {
			component.Value = values
		}
		return components, nil
	}

	if len(values) == 1 && (v.IsFontKeyword(values[0].Value) || v.IsGlobal(values[0].Value) || v.IsPrefixed(values[0].Value)) {
		values[0].Value = token.Internal + values[0].Value
		for _, component := range components {
			component.Value = values
		}
		return components, nil
	}

	if len(values) < 2 || !anyIsFontSize(values, v) || !anyIsFontFamily(values, v) {
		return nil, invalid("Invalid font values at " + namePositionOf(property) + ". Ignoring.")
	}

	if len(values) > 1 && anyIsInherit(values) {
		return nil, invalid("Invalid font values at " + positionOf(values[0]) + ". Ignoring.")
	}

	// fuzzy match style, variant, weight, and stretch on first elements
fuzzy:
	for index < fuzzyMatched && at(index) != nil {
		text := values[index].Value
		isStretchValid := v.IsFontStretchKeyword(text) || v.IsGlobal(text)
		isStyleValid := v.IsFontStyleKeyword(text) || v.IsGlobal(text)
		isVariantValid := v.IsFontVariantKeyword(text) || v.IsGlobal(text)
		isWeightValid := v.IsFontWeightKeyword(text) || v.IsGlobal(text)

		switch {
		case isStyleValid && !isStyleSet:
			style.Value = []*token.Token{values[index]}
			isStyleSet = true
		case isVariantValid && !isVariantSet:
			variant.Value = []*token.Token{values[index]}
			isVariantSet = true
		case isWeightValid && !isWeightSet:
			weight.Value = []*token.Token{values[index]}
			isWeightSet = true
		case isStretchValid && !isStretchSet:
			stretch.Value = []*token.Token{values[index]}
			isStretchSet = true
		case isStyleValid && isStyleSet || isVariantValid && isVariantSet || isWeightValid && isWeightSet || isStretchValid && isStretchSet:
			return nil, invalid("Invalid font style / variant / weight / stretch value at " + positionOf(values[0]) + ". Ignoring.")
		default:
			break fuzzy
		}

		index++
	}

	// now comes font-size ...
	if current := at(index); current != nil && (v.IsFontSizeKeyword(current.Value) || v.IsUnit(current.Value) && !v.IsDynamicUnit(current.Value)) {
		size.Value = []*token.Token{current}
		index++
	} else {
		return nil, invalid("Missing font size at " + positionOf(values[0]) + ". Ignoring.")
	}

	if at(index) == nil {
		return nil, invalid("Missing font family at " + positionOf(values[0]) + ". Ignoring.")
	}

	// ... and perhaps line-height
	if at(index) != nil && at(index).Value == "/" && at(index+1) != nil &&
		(v.IsLineHeightKeyword(at(index+1).Value) || v.IsUnit(at(index+1).Value) || v.IsNumber(at(index+1).Value)) {
		height.Value = []*token.Token{at(index + 1)}
		index += 2
	}

	// ... and whatever comes next is font-family
	family.Value = nil

	for at(index) != nil {
		if values[index].Value == "," {
			appendableFamilyName = false
		} else {
			if appendableFamilyName {
				family.Value[len(family.Value)-1].Value += " " + values[index].Value
			} else {
				family.Value = append(family.Value, values[index])
			}
			appendableFamilyName = true
		}
		index++
	}

	if len(family.Value) == 0 {
		return nil, invalid("Missing font family at " + positionOf(values[0]) + ". Ignoring.")
	}

	return components, nil
}

func anyIsFontSize(values []*token.Token, v *Validator) bool {
	for _, value := range values {
		if v.IsFontSizeKeyword(value.Value) || v.IsUnit(value.Value) && !v.IsDynamicUnit(value.Value) || v.IsFunction(value.Value) {
			return true
		}
	}
	return false
}

func anyIsFontFamily(values []*token.Token, v *Validator) bool {
	for _, value := range values {
		if v.IsIdentifier(value.Value) || v.IsQuotedText(value.Value) {
			return true
		}
	}
	return false
}

// cloneValue copies a value token the way slicing the original's token array does.
func cloneValue(t *token.Token) *token.Token {
	c := *t
	return &c
}

// fourValues expands a one to four value property into its four side
// components; the property's own value list is padded in place.
func fourValues(property *Property) []*Property {
	componentNames := configuration[property.Name].Components
	value := property.Value

	if len(value) < 1 {
		return []*Property{}
	}

	if len(value) < 2 {
		value = append(value, cloneValue(value[0]))
	}
	if len(value) < 3 {
		value = append(value, cloneValue(value[0]))
	}
	if len(value) < 4 {
		value = append(value, cloneValue(value[1]))
	}
	property.Value = value

	components := make([]*Property, len(componentNames))
	for i := len(componentNames) - 1; i >= 0; i-- {
		component := WrapSingle(token.NewProperty(componentNames[i]))
		component.Value = []*token.Token{value[i]}
		components[i] = component
	}
	return components
}

func breakUpFourValues(property *Property, _ *Validator) ([]*Property, error) {
	return fourValues(property), nil
}

// breakUpMultiplex breaks up a comma separated list of layers with splitWith
// and joins the components of each layer back with commas.
func breakUpMultiplex(splitWith BreakUp) BreakUp {
	return func(property *Property, v *Validator) ([]*Property, error) {
		var splitsAt []int
		values := property.Value

		// find split commas
		for i, value := range values {
			if value.Value == "," {
				splitsAt = append(splitsAt, i)
			}
		}

		if len(splitsAt) == 0 {
			return splitWith(property, v)
		}

		var splitComponents [][]*Property

		// split over commas, and into components
		for i := 0; i <= len(splitsAt); i++ {
			from := 0
			if i > 0 {
				from = splitsAt[i-1] + 1
			}
			to := len(values)
			if i < len(splitsAt) {
				to = splitsAt[i]
			}

			layer := WrapDefault(property.Name)
			layer.Value = token.Copy(values[from:to])

			if len(layer.Value) > 0 {
				components, err := splitWith(layer, v)
				if err != nil {
					return nil, err
				}
				splitComponents = append(splitComponents, components)
			}
		}

		if len(splitComponents) == 0 {
			panic("cannot read properties of undefined (reading 'length')")
		}
		components := splitComponents[0]

		// group component values from each split
		for i := range components {
			components[i].Multiplex = true

			for j := 1; j < len(splitComponents); j++ {
				components[i].Value = append(components[i].Value, token.NewValue(","))
				components[i].Value = append(components[i].Value, splitComponents[j][i].Value...)
			}
		}

		return components, nil
	}
}

func breakUpListStyle(property *Property, v *Validator) ([]*Property, error) {
	typ := WrapDefault("list-style-type")
	position := WrapDefault("list-style-position")
	image := WrapDefault("list-style-image")
	components := []*Property{typ, position, image}

	if len(property.Value) == 1 && property.Value[0].Value == "inherit" {
		typ.Value = []*token.Token{property.Value[0]}
		position.Value = []*token.Token{property.Value[0]}
		image.Value = []*token.Token{property.Value[0]}
		return components, nil
	}

	values := token.Copy(property.Value)

	// `image` first...
	for index, value := range values {
		if v.IsUrl(value.Value) || value.Value == "0" {
			image.Value = []*token.Token{value}
			values = slices.Delete(values, index, index+1)
			break
		}
	}

	// ... then `position`
	for index, value := range values {
		if v.IsListStylePositionKeyword(value.Value) {
			position.Value = []*token.Token{value}
			values = slices.Delete(values, index, index+1)
			break
		}
	}

	// ... and what's left is a `type`
	if len(values) > 0 && (v.IsListStyleTypeKeyword(values[0].Value) || v.IsIdentifier(values[0].Value)) {
		typ.Value = []*token.Token{values[0]}
	}

	return components, nil
}

func breakUpTransition(property *Property, v *Validator) ([]*Property, error) {
	prop := WrapDefault(property.Name + "-property")
	duration := WrapDefault(property.Name + "-duration")
	timing := WrapDefault(property.Name + "-timing-function")
	delay := WrapDefault(property.Name + "-delay")
	components := []*Property{prop, duration, timing, delay}
	values := property.Value
	var durationSet, delaySet, propSet, timingSet bool

	if len(values) == 1 && values[0].Value == "inherit" {
		for _, component := range components {
			component.Value = values
		}
		return components, nil
	}

	if len(values) > 1 && anyIsInherit(values) {
		return nil, invalid("Invalid animation values at " + positionOf(values[0]) + ". Ignoring.")
	}

	for _, value := range values {
		text := value.Value
		switch {
		case v.IsTime(text) && !durationSet:
			duration.Value = []*token.Token{value}
			durationSet = true
		case v.IsTime(text) && !delaySet:
			delay.Value = []*token.Token{value}
			delaySet = true
		case (v.IsGlobal(text) || v.IsTimingFunction(text)) && !timingSet:
			timing.Value = []*token.Token{value}
			timingSet = true
		case v.IsIdentifier(text) && !propSet:
			prop.Value = []*token.Token{value}
			propSet = true
		default:
			return nil, invalid("Invalid animation value at " + positionOf(value) + ". Ignoring.")
		}
	}

	return components, nil
}

// breakUpBorder breaks up width / style / color shorthands such as border
// and outline, accepting their parts in any order.
func breakUpBorder(property *Property, v *Validator) ([]*Property, error) {
	descriptor := configuration[property.Name]
	components := []*Property{
		WrapDefault(descriptor.Components[0]),
		WrapDefault(descriptor.Components[1]),
		WrapDefault(descriptor.Components[2]),
	}
	var color, style, width *Property

	for _, component := range components {
		switch {
		case strings.Index(component.Name, "color") > 0:
			color = component
		case strings.Index(component.Name, "style") > 0:
			style = component
		default:
			width = component
		}
	}

	if len(property.Value) == 1 && property.Value[0].Value == "inherit" ||
		len(property.Value) == 3 && property.Value[0].Value == "inherit" && property.Value[1].Value == "inherit" && property.Value[2].Value == "inherit" {
		color.Value = []*token.Token{property.Value[0]}
		style.Value = []*token.Token{property.Value[0]}
		width.Value = []*token.Token{property.Value[0]}
		return components, nil
	}

	values := token.Copy(property.Value)

	// NOTE: usually users don't follow the required order of parts in this shorthand,
	// so we'll try to parse it caring as little about order as possible

	if len(values) > 0 {
		matches := filterValues(values, widthFilter(v))
		var match *token.Token
		if len(matches) > 1 && (matches[0].Value == "none" || matches[0].Value == "auto") {
			match = matches[1]
		} else if len(matches) > 0 {
			match = matches[0]
		}
		if match != nil {
			width.Value = []*token.Token{match}
			values = removeToken(values, match)
		}
	}

	if len(values) > 0 {
		if matches := filterValues(values, styleFilter(v)); len(matches) > 0 {
			style.Value = []*token.Token{matches[0]}
			values = removeToken(values, matches[0])
		}
	}

	if len(values) > 0 {
		if matches := filterValues(values, colorFilter(v)); len(matches) > 0 {
			color.Value = []*token.Token{matches[0]}
			values = removeToken(values, matches[0])
		}
	}

	return components, nil
}

func removeToken(values []*token.Token, t *token.Token) []*token.Token {
	if i := slices.Index(values, t); i > -1 {
		return slices.Delete(values, i, i+1)
	}
	return values
}

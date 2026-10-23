package level2

import (
	"slices"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

// componentSet maps longhand names to the declarations that may form a
// shorthand, in the order they were seen.
type componentSet = jsutil.Object[*optimizer.Property]

// mergeIntoShorthands replaces complete sets of longhands with their
// shorthand. properties is changed in place: a merged shorthand replaces an
// entry and inherit-preserving longhands may be appended.
func mergeIntoShorthands(properties *[]*optimizer.Property, validator *optimizer.Validator) {
	candidates := jsutil.NewObject[*componentSet]()

	// there is no shorthand property made up of less than 3 longhands
	if len(*properties) < 3 {
		return
	}

	i := 0
	for l := len(*properties); i < l; i++ {
		property := (*properties)[i]
		descriptor := lookup(property.Name)

		if property.Dynamic || property.Unused || property.Hack != nil || property.Block {
			continue
		}

		if descriptor != nil && descriptor.SingleTypeComponents && !hasSameValues(property) {
			continue
		}

		invalidateOrCompact(properties, i, candidates, validator)

		if descriptor != nil && descriptor.ComponentOf != nil {
			for _, componentOf := range descriptor.ComponentOf {
				set, ok := candidates.Get(componentOf)
				if !ok || set == nil {
					set = jsutil.NewObject[*optimizer.Property]()
					candidates.Set(componentOf, set)
				}
				set.Set(property.Name, property)
			}
		}
	}

	invalidateOrCompact(properties, i, candidates, validator)
}

func invalidateOrCompact(properties *[]*optimizer.Property, position int, candidates *jsutil.Object[*componentSet], validator *optimizer.Validator) {
	var invalidatedBy *optimizer.Property
	if position < len(*properties) {
		invalidatedBy = (*properties)[position]
	}
	var replacedCandidates []string

	for _, shorthandName := range candidates.Keys() {
		candidateComponents, ok := candidates.Get(shorthandName)
		if !ok {
			continue
		}
		if invalidatedBy != nil && shorthandName == invalidatedBy.Name {
			continue
		}

		shorthandDescriptor := lookup(shorthandName)
		if shorthandDescriptor == nil {
			continue
		}
		if invalidatedBy != nil && invalidates(candidates, shorthandName, invalidatedBy) {
			candidates.Delete(shorthandName)
			continue
		}

		if len(shorthandDescriptor.Components) > candidateComponents.Len() {
			continue
		}
		if mixedImportance(candidateComponents) {
			continue
		}
		if !overridable(candidateComponents, shorthandName, validator) {
			continue
		}
		if !mergeable(candidateComponents) {
			continue
		}

		if mixedInherit(candidateComponents) {
			replaceWithInheritBestFit(properties, candidateComponents, shorthandName, validator)
		} else {
			replaceWithShorthand(properties, candidateComponents, shorthandName, validator)
		}

		replacedCandidates = append(replacedCandidates, shorthandName)
	}

	for i := len(replacedCandidates) - 1; i >= 0; i-- {
		candidates.Delete(replacedCandidates[i])
	}
}

func invalidates(candidates *jsutil.Object[*componentSet], shorthandName string, invalidatedBy *optimizer.Property) bool {
	shorthandDescriptor := lookup(shorthandName)
	invalidatedByDescriptor := lookup(invalidatedBy.Name)

	if shorthandDescriptor.HasOverridesShorthands && slices.Contains(shorthandDescriptor.OverridesShorthands, invalidatedBy.Name) {
		return true
	}

	if invalidatedByDescriptor != nil && invalidatedByDescriptor.ComponentOf != nil {
		components, _ := candidates.Get(shorthandName)
		for _, componentName := range components.Keys() {
			if slices.Contains(invalidatedByDescriptor.ComponentOf, componentName) {
				return true
			}
		}
	}

	return false
}

func componentsInOrder(components *componentSet) []*optimizer.Property {
	keys := components.Keys()
	list := make([]*optimizer.Property, len(keys))
	for i, key := range keys {
		list[i], _ = components.Get(key)
	}
	return list
}

func lastComponent(components *componentSet) *optimizer.Property {
	list := componentsInOrder(components)
	return list[len(list)-1]
}

func mixedImportance(components *componentSet) bool {
	var important, seen bool
	for _, component := range componentsInOrder(components) {
		if seen && component.Important != important {
			return true
		}
		important = component.Important
		seen = true
	}
	return false
}

// placeholder returns a new declaration of a shorthand with the given value.
func placeholder(shorthandName, value string) *token.Token {
	return token.NewProperty(shorthandName, token.NewValue(value))
}

// newShorthand wraps a placeholder declaration of a shorthand and breaks it
// up, so that its components can be filled in.
func newShorthand(placeholderToken *token.Token, validator *optimizer.Validator) *optimizer.Property {
	property := optimizer.WrapSingle(placeholderToken)
	var discarded []string
	populateComponents([]*optimizer.Property{property}, validator, &discarded)
	return property
}

func setComponent(property *optimizer.Property, i int, component *optimizer.Property) {
	for len(property.Components) <= i {
		property.Components = append(property.Components, nil)
	}
	property.Components[i] = component
}

func overridable(components *componentSet, shorthandName string, validator *optimizer.Validator) bool {
	descriptor := lookup(shorthandName)
	newProperty := newShorthand(placeholder(shorthandName, descriptor.Default()), validator)

	for i, name := range descriptor.Components {
		component, _ := components.Get(name)
		var placeholderComponent *optimizer.Property
		if i < len(newProperty.Components) {
			placeholderComponent = newProperty.Components[i]
		}
		if !everyValuesPair(overrideCheckFor(component.Name, validator), placeholderComponent, component) {
			return false
		}
	}

	return true
}

func mergeable(components *componentSet) bool {
	lastCount := -1

	for _, name := range components.Keys() {
		component, _ := components.Get(name)
		descriptor := lookup(name)
		if descriptor == nil || descriptor.Restore == nil {
			continue
		}

		currentCount := len(descriptor.Restore(component))
		if lastCount != -1 && currentCount != lastCount {
			return false
		}
		lastCount = currentCount
	}

	return true
}

func mixedInherit(components *componentSet) bool {
	var last, seen bool
	for _, component := range componentsInOrder(components) {
		current := hasInherit(component)
		if seen && last != current {
			return true
		}
		last = current
		seen = true
	}
	return false
}

func replaceWithInheritBestFit(properties *[]*optimizer.Property, candidateComponents *componentSet, shorthandName string, validator *optimizer.Validator) {
	longhandsSequence, longhandsProperty, longhandsComponents := buildSequenceWithInheritLonghands(candidateComponents, shorthandName, validator)
	shorthandSequence, shorthandProperty, shorthandComponents := buildSequenceWithInheritShorthand(candidateComponents, shorthandName, validator)

	isLonghandsShorter := jsutil.Len16(writer.Body(longhandsSequence)) < jsutil.Len16(writer.Body(shorthandSequence))
	newTokensSequence, newProperty, newComponents := shorthandSequence, shorthandProperty, shorthandComponents
	if isLonghandsShorter {
		newTokensSequence, newProperty, newComponents = longhandsSequence, longhandsProperty, longhandsComponents
	}

	last := lastComponent(candidateComponents)
	all := last.All
	insertAt := last.Position

	newProperty.Position = insertAt
	newProperty.Shorthand = true
	newProperty.Important = last.Important
	newProperty.Multiplex = false
	newProperty.Dirty = true
	newProperty.All = all
	(*all)[insertAt] = newTokensSequence[0]

	*properties = splice(*properties, insertAt, 1, newProperty)

	for _, componentName := range candidateComponents.Keys() {
		oldComponent, _ := candidateComponents.Get(componentName)
		oldComponent.Unused = true

		newProperty.Multiplex = newProperty.Multiplex || oldComponent.Multiplex

		if newComponent, ok := newComponents.Get(oldComponent.Name); ok {
			newToken := findTokenIn(newTokensSequence, componentName)

			newComponent.Position = len(*all)
			newComponent.All = all
			*all = append(*all, newToken)

			*properties = append(*properties, newComponent)
		}
	}
}

// longhandToken returns a new declaration with the name token of the
// component's declaration and the component's values.
func longhandToken(component *optimizer.Property) *token.Token {
	original := component.Token()
	return &token.Token{Kind: token.Property, Name: original.Name, Values: slices.Clone(component.Value)}
}

func buildSequenceWithInheritLonghands(components *componentSet, shorthandName string, validator *optimizer.Validator) ([]*token.Token, *optimizer.Property, *componentSet) {
	var tokensSequence []*token.Token
	inheritComponents := jsutil.NewObject[*optimizer.Property]()
	nonInheritComponents := jsutil.NewObject[*optimizer.Property]()
	descriptor := lookup(shorthandName)
	shorthandToken := placeholder(shorthandName, descriptor.Default())
	newProperty := newShorthand(shorthandToken, validator)

	for i, name := range descriptor.Components {
		component, _ := components.Get(name)

		if hasInherit(component) {
			tokensSequence = append(tokensSequence, longhandToken(component))

			newComponent := optimizer.DeepClone(component)
			newComponent.Value = inferComponentValue(components, newComponent.Name)
			setComponent(newProperty, i, newComponent)
			inheritComponents.Set(component.Name, optimizer.DeepClone(component))
		} else {
			newComponent := optimizer.DeepClone(component)
			newComponent.All = component.All
			setComponent(newProperty, i, newComponent)
			nonInheritComponents.Set(component.Name, component)
		}
	}

	newProperty.Important = lastComponent(components).Important

	shorthandToken.Name.Positions = joinMetadata(nonInheritComponents, false)

	optimizer.RestoreFromOptimizing([]*optimizer.Property{newProperty}, restoreWithComponents)

	shorthandToken = &token.Token{Kind: token.Property, Name: shorthandToken.Name, Values: slices.Clone(newProperty.Value)}
	tokensSequence = append([]*token.Token{shorthandToken}, tokensSequence...)

	return tokensSequence, newProperty, inheritComponents
}

func inferComponentValue(components *componentSet, propertyName string) []*token.Token {
	descriptor := lookup(propertyName)
	if descriptor.OppositeTo != "" {
		if opposite, ok := components.Get(descriptor.OppositeTo); ok {
			return opposite.Value
		}
		return nil
	}
	return []*token.Token{descriptor.DefaultToken()}
}

// joinMetadata collects the positions of the names (or of the first values)
// of the components' declarations, sorted by line and column.
func joinMetadata(components *componentSet, values bool) []token.Position {
	var metadata []token.Position

	for _, component := range componentsInOrder(components) {
		original := component.Token()
		part := original.Name
		if values {
			if len(original.Values) == 0 {
				continue
			}
			part = original.Values[0]
		}
		metadata = append(metadata, part.Positions...)
	}

	jsutil.Sort(metadata, metadataSorter)
	return metadata
}

func metadataSorter(metadata1, metadata2 token.Position) float64 {
	switch {
	case metadata1.Line < metadata2.Line:
		return -1
	case metadata1.Line == metadata2.Line:
		if metadata1.Column < metadata2.Column {
			return -1
		}
		return 1
	}
	return 1
}

func buildSequenceWithInheritShorthand(components *componentSet, shorthandName string, validator *optimizer.Validator) ([]*token.Token, *optimizer.Property, *componentSet) {
	var tokensSequence []*token.Token
	inheritComponents := jsutil.NewObject[*optimizer.Property]()
	nonInheritComponents := jsutil.NewObject[*optimizer.Property]()
	descriptor := lookup(shorthandName)
	shorthandToken := placeholder(shorthandName, "inherit")
	newProperty := newShorthand(shorthandToken, validator)

	for _, name := range descriptor.Components {
		component, _ := components.Get(name)

		if hasInherit(component) {
			inheritComponents.Set(component.Name, component)
		} else {
			tokensSequence = append(tokensSequence, longhandToken(component))
			nonInheritComponents.Set(component.Name, optimizer.DeepClone(component))
		}
	}

	shorthandToken.Name.Positions = joinMetadata(inheritComponents, false)
	shorthandToken.Values[0].Positions = joinMetadata(inheritComponents, true)

	tokensSequence = append([]*token.Token{shorthandToken}, tokensSequence...)

	return tokensSequence, newProperty, nonInheritComponents
}

func findTokenIn(tokens []*token.Token, componentName string) *token.Token {
	for _, t := range tokens {
		if t.Name.Value == componentName {
			return t
		}
	}
	return nil
}

func replaceWithShorthand(properties *[]*optimizer.Property, candidateComponents *componentSet, shorthandName string, validator *optimizer.Validator) {
	descriptor := lookup(shorthandName)
	newValuePlaceholder := placeholder(shorthandName, descriptor.Default())
	insertAt := inferInsertAtFrom(*properties, candidateComponents, shorthandName)

	newProperty := optimizer.WrapSingle(newValuePlaceholder)
	newProperty.Shorthand = true
	newProperty.Dirty = true
	newProperty.Multiplex = false

	var discarded []string
	populateComponents([]*optimizer.Property{newProperty}, validator, &discarded)

	var all *[]*token.Token
	for i, name := range descriptor.Components {
		component, _ := candidateComponents.Get(name)

		setComponent(newProperty, i, optimizer.DeepClone(component))
		newProperty.Important = component.Important
		newProperty.Multiplex = newProperty.Multiplex || component.Multiplex

		all = component.All
	}

	for _, component := range componentsInOrder(candidateComponents) {
		component.Unused = true
	}

	newValuePlaceholder.Name.Positions = joinMetadata(candidateComponents, false)
	newValuePlaceholder.Values[0].Positions = joinMetadata(candidateComponents, true)

	newProperty.Position = insertAt
	newProperty.All = all
	(*all)[insertAt] = newValuePlaceholder

	*properties = splice(*properties, insertAt, 1, newProperty)
}

func inferInsertAtFrom(properties []*optimizer.Property, candidateComponents *componentSet, shorthandName string) int {
	list := componentsInOrder(candidateComponents)
	firstCandidatePosition := list[0].Position
	lastCandidatePosition := list[len(list)-1].Position

	if shorthandName == "border" && traversesVia(sliceRange(properties, firstCandidatePosition, lastCandidatePosition), "border-image") {
		return firstCandidatePosition
	}
	return lastCandidatePosition
}

func traversesVia(properties []*optimizer.Property, propertyName string) bool {
	for i := len(properties) - 1; i >= 0; i-- {
		if properties[i].Name == propertyName {
			return true
		}
	}
	return false
}

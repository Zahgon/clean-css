package level2

import (
	"regexp"
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

func sortingMethod(ctx *optimizer.Context) any {
	return ctx.Options.Level.Options(options.One).Get("selectorsSortingMethod")
}

func adjacentSpace(ctx *optimizer.Context) bool {
	return ctx.Options.Compatibility.Bool("selectors", "adjacentSpace")
}

// mergeSelectors joins two selector lists into one tidied, sorted list.
func mergeSelectors(first, second []*token.Token, ctx *optimizer.Context) []*token.Token {
	joined := append(slices.Clone(first), second...)
	merged := optimizer.TidyRules(joined, false, adjacentSpace(ctx), nil, ctx)
	return optimizer.SortSelectors(merged, sortingMethod(ctx))
}

// mergeAdjacent merges neighbouring rules with the same selectors (joining
// their bodies) or the same bodies (joining their selectors).
func mergeAdjacent(tokens []*token.Token, ctx *optimizer.Context) {
	var last *token.Token
	m := mergeabilityOf(ctx)
	mergeLimit := ctx.Options.Compatibility.Get("selectors", "mergeLimit")

	for _, t := range tokens {
		if t.Kind != token.Rule {
			last = nil
			continue
		}

		switch {
		case last != nil && writer.Rules(t.Scopes) == writer.Rules(last.Scopes):
			last.Body = append(last.Body, t.Body...)
			optimizeProperties(&last.Body, true, true, ctx)
			t.Body = []*token.Token{}
		case last != nil && writer.Body(t.Body) == writer.Body(last.Body) &&
			m.isMergeable(writer.Rules(t.Scopes)) &&
			m.isMergeable(writer.Rules(last.Scopes)) &&
			jsutil.LessThan(float64(len(last.Scopes)), mergeLimit):
			last.Scopes = mergeSelectors(last.Scopes, t.Scopes, ctx)
			t.Body = []*token.Token{}
		default:
			last = t
		}
	}
}

var (
	unsafeSelectorPattern = regexp.MustCompile(`\.|\*| :`)
	modifierRunPattern    = regexp.MustCompile(`--[^ ,>+~:]+`)
)

func isBemElement(t *token.Token) bool {
	asString := writer.Rules(t.Scopes)
	return strings.Contains(asString, "__") || strings.Contains(asString, "--")
}

func withoutModifier(selector string) string {
	return modifierRunPattern.ReplaceAllLiteralString(selector, "")
}

func removeAnyUnsafeElements(left *token.Token, candidates *jsutil.Object[*token.Token]) {
	leftSelector := withoutModifier(writer.Rules(left.Scopes))

	for _, body := range candidates.Keys() {
		right, ok := candidates.Get(body)
		if !ok {
			continue
		}
		var rightSelector string
		if right != nil {
			rightSelector = withoutModifier(writer.Rules(right.Scopes))
		}
		if strings.Contains(rightSelector, leftSelector) || strings.Contains(leftSelector, rightSelector) {
			candidates.Delete(body)
		}
	}
}

// mergeNonAdjacentByBody merges rules with identical bodies that are not
// next to each other, walking backwards and stopping at unsafe selectors.
func mergeNonAdjacentByBody(tokens []*token.Token, ctx *optimizer.Context) {
	mergeSemantically := levelOptions(ctx).Bool("mergeSemantically")
	m := mergeabilityOf(ctx)
	candidates := jsutil.NewObject[*token.Token]()

	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		if t.Kind != token.Rule {
			continue
		}

		if len(t.Body) > 0 && !mergeSemantically && unsafeSelectorPattern.MatchString(writer.Rules(t.Scopes)) {
			candidates = jsutil.NewObject[*token.Token]()
		}

		if len(t.Body) > 0 && mergeSemantically && isBemElement(t) {
			removeAnyUnsafeElements(t, candidates)
		}

		candidateBody := writer.Body(t.Body)
		oldToken, _ := candidates.Get(candidateBody)
		if oldToken != nil && m.isMergeable(writer.Rules(t.Scopes)) && m.isMergeable(writer.Rules(oldToken.Scopes)) {
			if len(t.Body) > 0 {
				t.Scopes = mergeSelectors(oldToken.Scopes, t.Scopes, ctx)
			} else {
				t.Scopes = append(slices.Clone(oldToken.Scopes), t.Scopes...)
			}

			oldToken.Body = []*token.Token{}
			candidates.Set(candidateBody, nil)
		}

		candidates.Set(writer.Body(t.Body), t)
	}
}

// mergeNonAdjacentBySelector merges rules with the same selectors that are
// not next to each other when the declarations in between allow it.
func mergeNonAdjacentBySelector(tokens []*token.Token, ctx *optimizer.Context) {
	cache := specificities(ctx)
	allSelectors := map[string][]int{}
	var repeatedSelectors []string

	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Kind != token.Rule || len(tokens[i].Body) == 0 {
			continue
		}
		selector := writer.Rules(tokens[i].Scopes)
		allSelectors[selector] = append([]int{i}, allSelectors[selector]...)
		if len(allSelectors[selector]) == 2 {
			repeatedSelectors = append(repeatedSelectors, selector)
		}
	}

	for i := len(repeatedSelectors) - 1; i >= 0; i-- {
		positions := allSelectors[repeatedSelectors[i]]

	selectorIterator:
		for j := len(positions) - 1; j > 0; j-- {
			positionOne := positions[j-1]
			tokenOne := tokens[positionOne]
			positionTwo := positions[j]
			tokenTwo := tokens[positionTwo]

		directionIterator:
			for direction := 1; direction >= -1; direction -= 2 {
				topToBottom := direction == 1
				from, to, delta := positionTwo-1, positionOne, -1
				moved, target := tokenTwo, tokenOne
				if topToBottom {
					from, to, delta = positionOne+1, positionTwo, 1
					moved, target = tokenOne, tokenTwo
				}
				movedProperties := extractProperties(moved)

				for from != to {
					traversedProperties := extractProperties(tokens[from])
					from += delta

					var reorderable bool
					if topToBottom {
						reorderable = canReorder(movedProperties, traversedProperties, cache)
					} else {
						reorderable = canReorder(traversedProperties, movedProperties, cache)
					}

					if !reorderable && !topToBottom {
						continue selectorIterator
					}
					if !reorderable && topToBottom {
						continue directionIterator
					}
				}

				if topToBottom {
					moved.Body = append(moved.Body, target.Body...)
					target.Body = moved.Body
				} else {
					target.Body = append(target.Body, moved.Body...)
				}

				optimizeProperties(&target.Body, true, true, ctx)
				moved.Body = []*token.Token{}
			}
		}
	}
}

// mergeMediaQueries merges nested blocks with the same scope when the
// declarations between them allow moving one onto the other; it returns the
// blocks that received content.
func mergeMediaQueries(tokens []*token.Token, ctx *optimizer.Context) []*token.Token {
	mergeSemantically := levelOptions(ctx).Bool("mergeSemantically")
	cache := specificities(ctx)
	candidates := jsutil.NewObject[*[]int]()
	var reduced []*token.Token

	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		if t.Kind != token.NestedBlock {
			continue
		}
		key := writer.Rules(t.Scopes)
		candidate, ok := candidates.Get(key)
		if !ok {
			candidate = &[]int{}
			candidates.Set(key, candidate)
		}
		*candidate = append(*candidate, i)
	}

	for _, name := range candidates.Keys() {
		positionsRef, _ := candidates.Get(name)
		positions := *positionsRef

	positionLoop:
		for j := len(positions) - 1; j > 0; j-- {
			positionOne := positions[j]
			tokenOne := tokens[positionOne]
			positionTwo := positions[j-1]
			tokenTwo := tokens[positionTwo]

		directionLoop:
			for direction := 1; direction >= -1; direction -= 2 {
				topToBottom := direction == 1
				from, to, delta := positionTwo-1, positionOne, -1
				source, target := tokenTwo, tokenOne
				if topToBottom {
					from, to, delta = positionOne+1, positionTwo, 1
					source, target = tokenOne, tokenTwo
				}
				movedProperties := extractProperties(source)

				for from != to {
					traversedProperties := extractProperties(tokens[from])
					from += delta

					if mergeSemantically && allSameRulePropertiesCanBeReordered(movedProperties, traversedProperties, cache) {
						continue
					}
					if !canReorder(movedProperties, traversedProperties, cache) {
						continue directionLoop
					}
				}

				if topToBottom {
					target.Body = append(slices.Clone(source.Body), target.Body...)
				} else {
					target.Body = append(slices.Clone(target.Body), source.Body...)
				}
				source.Body = []*token.Token{}

				reduced = append(reduced, target)
				continue positionLoop
			}
		}
	}

	return reduced
}

func allSameRulePropertiesCanBeReordered(movedProperties, traversedProperties []*extractedProperty, cache map[string][3]int) bool {
	for _, moved := range movedProperties {
		for _, traversed := range traversedProperties {
			if rulesOverlap(moved.selector, traversed.selector, true) && !canReorderSingle(moved, traversed, cache) {
				return false
			}
		}
	}
	return true
}

// removeDuplicates empties earlier rules whose selectors and body repeat a
// later rule.
func removeDuplicates(tokens []*token.Token) {
	matched := map[string][]int{}
	var moreThanOnce []string

	for i, t := range tokens {
		if t.Kind != token.Rule {
			continue
		}
		id := writer.Rules(t.Scopes)
		if len(matched[id]) == 1 {
			moreThanOnce = append(moreThanOnce, id)
		}
		matched[id] = append(matched[id], i)
	}

	for _, id := range moreThanOnce {
		var bodies []string
		for j := len(matched[id]) - 1; j >= 0; j-- {
			t := tokens[matched[id][j]]
			body := writer.Body(t.Body)
			if slices.Contains(bodies, body) {
				t.Body = []*token.Token{}
			} else {
				bodies = append(bodies, body)
			}
		}
	}
}

// removeDuplicateFontAtRules empties repeated @font-face (and other
// at-rule) blocks.
func removeDuplicateFontAtRules(tokens []*token.Token) {
	var fontAtRules []string

	for _, t := range tokens {
		if t.Kind != token.AtRuleBlock && !(len(t.Scopes) > 0 && t.Scopes[0].Value == "@font-face") {
			continue
		}

		key := writer.All([]*token.Token{t})
		if slices.Contains(fontAtRules, key) {
			t.Body = []*token.Token{}
		} else {
			fontAtRules = append(fontAtRules, key)
		}
	}
}

// removeDuplicateMediaQueries empties nested blocks repeated later with the
// same scope and content.
func removeDuplicateMediaQueries(tokens []*token.Token) {
	candidates := map[string]*token.Token{}

	for _, t := range tokens {
		if t.Kind != token.NestedBlock {
			continue
		}
		key := writer.Rules(t.Scopes) + "%" + writer.All(t.Body)
		if candidate := candidates[key]; candidate != nil {
			candidate.Body = []*token.Token{}
		}
		candidates[key] = t
	}
}

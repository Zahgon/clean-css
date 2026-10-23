package level2

import (
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

const idJoinCharacter = "%"

const maxCombinationsLevel = 2

// fit is a set of rules a declaration could be moved out of, with the size
// difference moving it would make (negative is a saving).
type fit struct {
	tokens     []*token.Token
	difference int
}

// multiMove is a declaration waiting in the multi-property move cache with
// the rules it could be moved out of.
type multiMove struct {
	property *extractedProperty
	tokens   []*token.Token
}

// restructurer moves declarations shared by several rules into new rules
// with all those selectors when that makes the output shorter.
type restructurer struct {
	tokens           *[]*token.Token
	ctx              *optimizer.Context
	mergeability     mergeability
	mergeLimit       any
	cache            map[string][3]int
	movableTokens    *jsutil.Object[[]*token.Token]
	movedProperties  []*extractedProperty
	moveCache        *jsutil.Object[[]multiMove]
	movedToBeDropped []int
}

func restructure(tokens *[]*token.Token, ctx *optimizer.Context) {
	r := &restructurer{
		tokens:        tokens,
		ctx:           ctx,
		mergeability:  mergeabilityOf(ctx),
		mergeLimit:    ctx.Options.Compatibility.Get("selectors", "mergeLimit"),
		cache:         specificities(ctx),
		movableTokens: jsutil.NewObject[[]*token.Token](),
		moveCache:     jsutil.NewObject[[]multiMove](),
	}
	r.run()
}

func (r *restructurer) sendToMultiPropertyMoveCache(position int, movedProperty *extractedProperty, allFits []fit) {
	for i := len(allFits) - 1; i >= 0; i-- {
		id := r.addToCache(movedProperty, allFits[i].tokens)
		cached, _ := r.moveCache.Get(id)
		if len(cached) > 1 && r.processMultiPropertyMove(position, cached) {
			r.removeAllMatchingFromCache(id)
			break
		}
	}
}

func (r *restructurer) addToCache(movedProperty *extractedProperty, tokens []*token.Token) string {
	id := cacheID(tokens)
	cached, _ := r.moveCache.Get(id)
	r.moveCache.Set(id, append(cached, multiMove{movedProperty, tokens}))
	return id
}

func (r *restructurer) removeAllMatchingFromCache(matchID string) {
	matchSelectors := strings.Split(matchID, idJoinCharacter)
	var forRemoval []string

	for _, id := range r.moveCache.Keys() {
		selectors := strings.Split(id, idJoinCharacter)
		for i := len(selectors) - 1; i >= 0; i-- {
			if slices.Contains(matchSelectors, selectors[i]) {
				forRemoval = append(forRemoval, id)
				break
			}
		}
	}

	for i := len(forRemoval) - 1; i >= 0; i-- {
		r.moveCache.Delete(forRemoval[i])
	}
}

func cacheID(tokens []*token.Token) string {
	id := make([]string, len(tokens))
	for i, t := range tokens {
		id[i] = writer.Rules(t.Scopes)
	}
	return strings.Join(id, idJoinCharacter)
}

func (r *restructurer) tokensToMerge(sourceTokens []*token.Token) []*token.Token {
	var uniqueTokensWithBody []*token.Token
	var mergeableTokens []*token.Token

	for i := len(sourceTokens) - 1; i >= 0; i-- {
		if !r.mergeability.isMergeable(writer.Rules(sourceTokens[i].Scopes)) {
			continue
		}
		mergeableTokens = append([]*token.Token{sourceTokens[i]}, mergeableTokens...)
		if len(sourceTokens[i].Body) > 0 && !containsToken(uniqueTokensWithBody, sourceTokens[i]) {
			uniqueTokensWithBody = append(uniqueTokensWithBody, sourceTokens[i])
		}
	}

	if len(uniqueTokensWithBody) > 1 {
		return mergeableTokens
	}
	return nil
}

func (r *restructurer) shortenIfPossible(position int, movedProperty *extractedProperty) {
	valueSize := jsutil.Len16(movedProperty.name) + jsutil.Len16(movedProperty.value) + 1

	movable, _ := r.movableTokens.Get(movedProperty.key)
	mergeableTokens := r.tokensToMerge(movable)
	if len(mergeableTokens) < 2 {
		return
	}

	allFits := r.findAllFits(mergeableTokens, valueSize, 1)
	bestFit := allFits[0]
	if bestFit.difference > 0 {
		r.sendToMultiPropertyMoveCache(position, movedProperty, allFits)
		return
	}

	allSelectors, qualifiedTokens := selectorsOf(bestFit)
	r.dropAsNewTokenAt(position, []*extractedProperty{movedProperty}, tidyRuleDuplicates(allSelectors), qualifiedTokens)
}

func selectorsOf(f fit) ([]*token.Token, []*token.Token) {
	var allSelectors []*token.Token
	var qualifiedTokens []*token.Token
	for i := len(f.tokens) - 1; i >= 0; i-- {
		allSelectors = append(slices.Clone(f.tokens[i].Scopes), allSelectors...)
		qualifiedTokens = append([]*token.Token{f.tokens[i]}, qualifiedTokens...)
	}
	return allSelectors, qualifiedTokens
}

func fitSorter(fit1, fit2 fit) float64 {
	switch {
	case fit1.difference > fit2.difference:
		return 1
	case fit1.difference == fit2.difference:
		return 0
	}
	return -1
}

func (r *restructurer) findAllFits(mergeableTokens []*token.Token, propertySize, propertiesCount int) []fit {
	combinations := r.allCombinations(mergeableTokens, propertySize, propertiesCount, maxCombinationsLevel-1)
	jsutil.Sort(combinations, fitSorter)
	return combinations
}

func (r *restructurer) allCombinations(variant []*token.Token, propertySize, propertiesCount, level int) []fit {
	differenceVariants := []fit{{variant, sizeDifference(variant, propertySize, propertiesCount)}}
	if len(variant) > 2 && level > 0 {
		for i := len(variant) - 1; i >= 0; i-- {
			subVariant := splice(slices.Clone(variant), i, 1)
			differenceVariants = append(differenceVariants, r.allCombinations(subVariant, propertySize, propertiesCount, level-1)...)
		}
	}
	return differenceVariants
}

func sizeDifference(variant []*token.Token, propertySize, propertiesCount int) int {
	allSelectorsSize := 0
	for i := len(variant) - 1; i >= 0; i-- {
		if len(variant[i].Body) > propertiesCount {
			allSelectorsSize += jsutil.Len16(writer.Rules(variant[i].Scopes))
		} else {
			allSelectorsSize--
		}
	}
	return allSelectorsSize - (len(variant)-1)*propertySize + 1
}

// nameOf returns what the original reads as the name of a body token: the
// property name, or the second character of a leaf token's text.
func nameOf(t *token.Token) string {
	if t.Kind == token.Property {
		return t.Name.Value
	}
	if len(t.Scopes) == 0 && len(t.Value) > 1 {
		return t.Value[1:2]
	}
	return ""
}

func (r *restructurer) dropAsNewTokenAt(position int, properties []*extractedProperty, allSelectors, mergeableTokens []*token.Token) {
	for i := len(mergeableTokens) - 1; i >= 0; i-- {
		mergeableToken := mergeableTokens[i]

		for j := len(mergeableToken.Body) - 1; j >= 0; j-- {
			mergeableProperty := mergeableToken.Body[j]

			for _, property := range properties {
				if nameOf(mergeableProperty) == property.name &&
					writer.Body([]*token.Token{mergeableProperty}) == property.key {
					mergeableToken.Body = splice(mergeableToken.Body, j, 1)
					break
				}
			}
		}
	}

	allProperties := make([]*token.Token, len(properties))
	for i, property := range properties {
		allProperties[i] = property.token
	}

	newToken := &token.Token{Kind: token.Rule, Scopes: allSelectors, Body: allProperties}
	*r.tokens = splice(*r.tokens, position, 0, newToken)
}

func (r *restructurer) dropPropertiesAt(position int, movedProperty *extractedProperty) {
	toMove, _ := r.movableTokens.Get(movedProperty.key)
	if len(toMove) > 1 {
		if !r.shortenMultiMovesIfPossible(position, movedProperty) {
			r.shortenIfPossible(position, movedProperty)
		}
	}
}

// shortenMultiMovesIfPossible moves several declarations shared by the same
// rules at once and reports whether it did.
func (r *restructurer) shortenMultiMovesIfPossible(position int, movedProperty *extractedProperty) bool {
	movable, _ := r.movableTokens.Get(movedProperty.key)
	mergeableTokens := r.tokensToMerge(movable)
	if len(mergeableTokens) < 2 {
		return false
	}

	var candidates []string
movableLoop:
	for _, value := range r.movableTokens.Keys() {
		tokensList, _ := r.movableTokens.Get(value)
		for j := len(mergeableTokens) - 1; j >= 0; j-- {
			if !containsToken(tokensList, mergeableTokens[j]) {
				continue movableLoop
			}
		}
		candidates = append(candidates, value)
	}

	if len(candidates) < 2 {
		return false
	}

	var propertiesAndMergeableTokens []multiMove
	for j := len(candidates) - 1; j >= 0; j-- {
		for k := len(r.movedProperties) - 1; k >= 0; k-- {
			if r.movedProperties[k].key == candidates[j] {
				propertiesAndMergeableTokens = append([]multiMove{{r.movedProperties[k], mergeableTokens}}, propertiesAndMergeableTokens...)
				break
			}
		}
	}

	return r.processMultiPropertyMove(position, propertiesAndMergeableTokens)
}

func (r *restructurer) processMultiPropertyMove(position int, moves []multiMove) bool {
	if len(moves) == 0 {
		return false
	}
	valueSize := 0
	var properties []*extractedProperty

	for i := len(moves) - 1; i >= 0; i-- {
		property := moves[i].property
		valueSize += jsutil.Len16(property.key)
		if i > 0 {
			valueSize++
		}
		properties = append(properties, property)
	}

	bestFit := r.findAllFits(moves[0].tokens, valueSize, len(properties))[0]
	if bestFit.difference > 0 {
		return false
	}

	allSelectors, qualifiedTokens := selectorsOf(bestFit)
	r.dropAsNewTokenAt(position, properties, tidyRuleDuplicates(allSelectors), qualifiedTokens)

	for i := len(properties) - 1; i >= 0; i-- {
		property := properties[i]
		index := slices.Index(r.movedProperties, property)

		r.movableTokens.Delete(property.key)

		if index > -1 && !slices.Contains(r.movedToBeDropped, index) {
			r.movedToBeDropped = append(r.movedToBeDropped, index)
		}
	}

	return true
}

func (r *restructurer) boundToAnotherPropertyInCurrentToken(property, movedProperty *extractedProperty, t *token.Token) bool {
	if property.name != movedProperty.name {
		return false
	}
	toMove, ok := r.movableTokens.Get(movedProperty.key)
	return ok && containsToken(toMove, t)
}

func cloneAndMergeSelectors(propertyA, propertyB *extractedProperty) *extractedProperty {
	cloned := propertyA.clone()
	cloned.selector = append(cloned.selector, propertyB.selector...)
	return cloned
}

func (r *restructurer) run() {
	tokens := r.tokens
	samePropertyAt := 0

	for i := len(*tokens) - 1; i >= 0; i-- {
		t := (*tokens)[i]
		var isRule bool

		switch t.Kind {
		case token.Rule:
			isRule = true
		case token.NestedBlock:
			isRule = false
		default:
			continue
		}

		// movedProperties may change in the loop below
		movedCount := len(r.movedProperties)

		properties := extractProperties(t)
		r.movedToBeDropped = nil

		var unmovableInCurrentToken []int
		for j := len(properties) - 1; j >= 0; j-- {
			for k := j - 1; k >= 0; k-- {
				if !canReorderSingle(properties[j], properties[k], r.cache) {
					unmovableInCurrentToken = append(unmovableInCurrentToken, j)
					break
				}
			}
		}

		for j := len(properties) - 1; j >= 0; j-- {
			property := properties[j]
			movedSameProperty := false

			for k := 0; k < movedCount && k < len(r.movedProperties); k++ {
				movedProperty := r.movedProperties[k]

				movable, hasMovable := r.movableTokens.Get(movedProperty.key)
				if !slices.Contains(r.movedToBeDropped, k) &&
					(!canReorderSingle(property, movedProperty, r.cache) && !r.boundToAnotherPropertyInCurrentToken(property, movedProperty, t) ||
						hasMovable && jsutil.StrictEqual(float64(len(movable)), r.mergeLimit)) {
					r.dropPropertiesAt(i+1, movedProperty)

					if !slices.Contains(r.movedToBeDropped, k) {
						r.movedToBeDropped = append(r.movedToBeDropped, k)
						r.movableTokens.Delete(movedProperty.key)
					}
				}

				if !movedSameProperty {
					movedSameProperty = property.name == movedProperty.name && property.value == movedProperty.value
					if movedSameProperty {
						samePropertyAt = k
					}
				}
			}

			if !isRule || slices.Contains(unmovableInCurrentToken, j) {
				continue
			}

			key := property.key

			if movedSameProperty && jsutil.LessThan(r.mergeLimit, float64(len(r.movedProperties[samePropertyAt].selector)+len(property.selector))) {
				r.dropPropertiesAt(i+1, r.movedProperties[samePropertyAt])
				r.movedProperties = splice(r.movedProperties, samePropertyAt, 1)
				r.movableTokens.Set(key, []*token.Token{t})
				movedSameProperty = false
			} else {
				movable, _ := r.movableTokens.Get(key)
				r.movableTokens.Set(key, append(movable, t))
			}

			if movedSameProperty {
				r.movedProperties[samePropertyAt] = cloneAndMergeSelectors(r.movedProperties[samePropertyAt], property)
			} else {
				r.movedProperties = append(r.movedProperties, property)
			}
		}

		jsutil.Sort(r.movedToBeDropped, func(a, b int) float64 {
			if a > b {
				return 1
			}
			return -1
		})
		for j, dropAt := range r.movedToBeDropped {
			r.movedProperties = splice(r.movedProperties, dropAt-j, 1)
		}
	}

	position := 0
	if len(*tokens) > 0 && (*tokens)[0].Kind == token.AtRule && strings.HasPrefix((*tokens)[0].Value, "@charset") {
		position = 1
	}
	for ; position < len(*tokens)-1; position++ {
		current := (*tokens)[position]
		isImportRule := current.Kind == token.AtRule && strings.HasPrefix(current.Value, "@import")
		isComment := current.Kind == token.Comment
		if !(isImportRule || isComment) {
			break
		}
	}

	for i := 0; i < len(r.movedProperties); i++ {
		r.dropPropertiesAt(position, r.movedProperties[i])
	}
}

func ruleSorter(s1, s2 *token.Token) float64 {
	if jsutil.CompareUTF16(s1.Value, s2.Value) > 0 {
		return 1
	}
	return -1
}

// tidyRuleDuplicates removes repeated selectors and sorts the rest.
func tidyRuleDuplicates(rules []*token.Token) []*token.Token {
	var list []*token.Token
	var repeated []string

	for _, rule := range rules {
		if !slices.Contains(repeated, rule.Value) {
			repeated = append(repeated, rule.Value)
			list = append(list, rule)
		}
	}

	jsutil.Sort(list, ruleSorter)
	return list
}

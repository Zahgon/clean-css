package level2

import (
	"slices"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

// reduceCandidate is one occurrence of a selector: the rule at where, and
// for selector lists the single selectors it consists of.
type reduceCandidate struct {
	where     int
	list      []string
	isPartial bool
	isComplex bool
}

// reduceNonAdjacent removes declarations from earlier rules that later
// rules with the same selectors override.
func reduceNonAdjacent(tokens []*token.Token, ctx *optimizer.Context) {
	m := mergeabilityOf(ctx)
	candidates := jsutil.NewObject[*[]reduceCandidate]()
	var repeated []string

	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		if t.Kind != token.Rule || len(t.Body) == 0 {
			continue
		}

		selectorAsString := writer.Rules(t.Scopes)
		isComplexAndNotSpecial := len(t.Scopes) > 1 && m.isMergeable(selectorAsString)
		wrappedSelectors := make([]string, len(t.Scopes))
		for k, scope := range t.Scopes {
			wrappedSelectors[k] = scope.Value
		}
		selectors := []string{selectorAsString}
		if isComplexAndNotSpecial {
			selectors = append(selectors, wrappedSelectors...)
		}

		for j, selector := range selectors {
			list, ok := candidates.Get(selector)
			if !ok {
				list = &[]reduceCandidate{}
				candidates.Set(selector, list)
			} else {
				repeated = append(repeated, selector)
			}
			*list = append(*list, reduceCandidate{
				where:     i,
				list:      wrappedSelectors,
				isPartial: isComplexAndNotSpecial && j > 0,
				isComplex: isComplexAndNotSpecial && j == 0,
			})
		}
	}

	reduceSimpleNonAdjacentCases(tokens, repeated, candidates, ctx)
	reduceComplexNonAdjacentCases(tokens, candidates, m, ctx)
}

func reduceSimpleNonAdjacentCases(tokens []*token.Token, repeated []string, candidates *jsutil.Object[*[]reduceCandidate], ctx *optimizer.Context) {
	for _, selector := range repeated {
		dataRef, _ := candidates.Get(selector)
		data := *dataRef

		reduceSelector(tokens, data,
			func(idx int, bodies []*token.Token) bool {
				return data[idx].isPartial && len(bodies) == 0
			},
			func(t *token.Token, newBody []*token.Token, processedCount, tokenIdx int) {
				if !data[processedCount-tokenIdx-1].isPartial {
					t.Body = newBody
				}
			}, ctx)
	}
}

func reduceComplexNonAdjacentCases(tokens []*token.Token, candidates *jsutil.Object[*[]reduceCandidate], m mergeability, ctx *optimizer.Context) {
allSelectors:
	for _, complexSelector := range candidates.Keys() {
		intoRef, ok := candidates.Get(complexSelector)
		if !ok {
			continue
		}
		into := *intoRef
		if !into[0].isComplex {
			continue
		}

		intoPosition := into[len(into)-1].where
		intoToken := tokens[intoPosition]
		var reducedBodies [][]*token.Token

		selectors := []string{complexSelector}
		if m.isMergeable(complexSelector) {
			selectors = into[0].list
		}

		for _, selector := range selectors {
			dataRef, _ := candidates.Get(selector)
			data := *dataRef

			if len(data) < 2 {
				continue allSelectors
			}

			reduceSelector(tokens, data,
				func(idx int, _ []*token.Token) bool {
					return data[idx].where < intoPosition
				},
				func(_ *token.Token, newBody []*token.Token, _, tokenIdx int) {
					if tokenIdx == 0 {
						reducedBodies = append(reducedBodies, newBody)
					}
				}, ctx)

			if writer.Body(reducedBodies[len(reducedBodies)-1]) != writer.Body(reducedBodies[0]) {
				continue allSelectors
			}
		}

		intoToken.Body = reducedBodies[0]
	}
}

// reduceSelector optimizes the combined bodies of the occurrences of a
// selector, then hands each occurrence the part of the result that is left
// for it.
func reduceSelector(tokens []*token.Token, data []reduceCandidate,
	filterOut func(idx int, bodies []*token.Token) bool,
	callback func(t *token.Token, newBody []*token.Token, processedCount, tokenIdx int),
	ctx *optimizer.Context) {
	var bodies []*token.Token
	var bodiesAsList [][]*token.Token
	var processedTokens []int

	for j := len(data) - 1; j >= 0; j-- {
		if filterOut(j, bodies) {
			continue
		}
		where := data[j].where
		clonedBody := token.CloneAll(tokens[where].Body)
		bodies = append(slices.Clone(bodies), clonedBody...)
		bodiesAsList = append(bodiesAsList, clonedBody)
		processedTokens = append(processedTokens, where)
	}

	optimizeProperties(&bodies, true, false, ctx)

	processedCount := len(processedTokens)
	propertyIdx := len(bodies) - 1
	tokenIdx := processedCount - 1

	for tokenIdx >= 0 {
		if (tokenIdx == 0 || propertyIdx >= 0 && propertyIdx < len(bodies) && containsToken(bodiesAsList[tokenIdx], bodies[propertyIdx])) && propertyIdx > -1 {
			propertyIdx--
			continue
		}

		newBody := slices.Clone(bodies[min(propertyIdx+1, len(bodies)):])
		bodies = bodies[:min(propertyIdx+1, len(bodies))]
		callback(tokens[processedTokens[tokenIdx]], newBody, processedCount, tokenIdx)

		tokenIdx--
	}
}

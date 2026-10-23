// Package level2 implements the level 2 optimizations: merging, reducing and
// restructuring rules across a stylesheet, merging longhands into
// shorthands, overriding properties and removing duplicate or unused blocks.
package level2

import (
	"regexp"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
)

var keyframesPattern = regexp.MustCompile(`@(-moz-|-o-|-webkit-)?keyframes`)

// Optimize runs the level 2 optimizations over a list of block-level tokens
// and returns the resulting list. Restructuring is skipped when
// withRestructuring is false, as it is inside keyframes and merged media blocks.
func Optimize(tokens []*token.Token, ctx *optimizer.Context, withRestructuring bool) []*token.Token {
	if ctx.Specificities == nil {
		ctx.Specificities = map[string][3]int{}
	}
	optimize(&tokens, ctx, withRestructuring)
	return tokens
}

func levelOptions(ctx *optimizer.Context) options.Values {
	return ctx.Options.Level.Options(options.Two)
}

func optimize(tokens *[]*token.Token, ctx *optimizer.Context, withRestructuring bool) {
	opts := levelOptions(ctx)

	optimizeBlocks(*tokens, ctx)
	optimizeRuleProperties(*tokens, ctx)

	if opts.Bool("removeDuplicateRules") {
		removeDuplicates(*tokens)
	}
	if opts.Bool("mergeAdjacentRules") {
		mergeAdjacent(*tokens, ctx)
	}
	if opts.Bool("reduceNonAdjacentRules") {
		reduceNonAdjacent(*tokens, ctx)
	}

	mergeNonAdjacent := opts.Get("mergeNonAdjacentRules")
	if jsutil.Truthy(mergeNonAdjacent) && !jsutil.LooseEqual(mergeNonAdjacent, "body") {
		mergeNonAdjacentBySelector(*tokens, ctx)
	}
	if jsutil.Truthy(mergeNonAdjacent) && !jsutil.LooseEqual(mergeNonAdjacent, "selector") {
		mergeNonAdjacentByBody(*tokens, ctx)
	}

	if opts.Bool("restructureRules") && opts.Bool("mergeAdjacentRules") && withRestructuring {
		restructure(tokens, ctx)
		mergeAdjacent(*tokens, ctx)
	}
	if opts.Bool("restructureRules") && !opts.Bool("mergeAdjacentRules") && withRestructuring {
		restructure(tokens, ctx)
	}

	if opts.Bool("removeDuplicateFontRules") {
		removeDuplicateFontAtRules(*tokens)
	}
	if opts.Bool("removeDuplicateMediaBlocks") {
		removeDuplicateMediaQueries(*tokens)
	}
	if opts.Bool("removeUnusedAtRules") {
		removeUnusedAtRules(*tokens, ctx)
	}

	if opts.Bool("mergeMedia") {
		reduced := mergeMediaQueries(*tokens, ctx)
		for i := len(reduced) - 1; i >= 0; i-- {
			optimize(&reduced[i].Body, ctx, false)
		}
	}

	if opts.Bool("removeEmpty") {
		removeEmpty(tokens)
	}
}

// optimizeBlocks optimizes the content of nested blocks first; keyframes are
// never restructured.
func optimizeBlocks(tokens []*token.Token, ctx *optimizer.Context) {
	for _, t := range tokens {
		if t.Kind != token.NestedBlock {
			continue
		}
		isKeyframes := len(t.Scopes) > 0 && keyframesPattern.MatchString(t.Scopes[0].Value)
		optimize(&t.Body, ctx, !isKeyframes)
	}
}

func optimizeRuleProperties(tokens []*token.Token, ctx *optimizer.Context) {
	for _, t := range tokens {
		switch t.Kind {
		case token.Rule:
			optimizeProperties(&t.Body, true, true, ctx)
		case token.NestedBlock:
			optimizeRuleProperties(t.Body, ctx)
		}
	}
}

// removeEmpty drops rules, blocks and at-rules left without content.
func removeEmpty(tokens *[]*token.Token) {
	for i := 0; i < len(*tokens); i++ {
		t := (*tokens)[i]
		isEmpty := false

		switch t.Kind {
		case token.Rule:
			isEmpty = len(t.Scopes) == 0 || len(t.Body) == 0
		case token.NestedBlock:
			removeEmpty(&t.Body)
			isEmpty = len(t.Body) == 0
		case token.AtRule:
			isEmpty = t.Value == ""
		case token.AtRuleBlock:
			isEmpty = len(t.Body) == 0
		}

		if isEmpty {
			*tokens = splice(*tokens, i, 1)
			i--
		}
	}
}

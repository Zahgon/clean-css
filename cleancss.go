/**
 * Clean-css - https://github.com/clean-css/clean-css
 * Released under the terms of MIT license
 *
 * Go port of lib/clean.js.
 */

// Package cleancss is a CSS minifier: it tokenizes stylesheets, inlines
// local @import rules, applies level 0, 1 and 2 optimizations and writes the
// result compactly or in a chosen format.
package cleancss

import (
	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/optimizer/level2"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/reader"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

// implicitRebaseToWarning is reported when URLs are rebased without an
// explicit target directory.
const implicitRebaseToWarning = "You have set `rebase: true` without giving `rebaseTo` option, which, in this case, defaults to the current working directory. " +
	"You are then warned this can lead to unexpected URL rebasing (aka here be dragons)! " +
	"If you are OK with the clean-css output, then you can get rid of this warning by giving clean-css a `rebaseTo: process.cwd()` option."

// Minifier minifies stylesheets with one set of options. The options keep
// state between minifications, as in the original: special comments counted
// against the specialComments limit stay counted for later inputs.
type Minifier struct {
	options *options.Options
}

// New returns a minifier for the given options object (nil for the
// defaults). Option values keep the loose typing they have in JavaScript.
func New(raw *jsutil.Object[any]) (*Minifier, error) {
	opts, err := options.From(raw)
	if err != nil {
		return nil, err
	}
	return &Minifier{options: opts}, nil
}

// Batch reports whether the batch option is set: array and hash inputs are
// then minified one source at a time.
func (m *Minifier) Batch() bool {
	return m.options.Batch
}

// Stats describes a minification. Sizes are lengths in UTF-16 code units.
type Stats struct {
	OriginalSize int
	MinifiedSize int
	// Efficiency is 1 - MinifiedSize/OriginalSize (NaN for empty input).
	Efficiency float64
}

// Output is the result of a minification.
type Output struct {
	Styles             string
	Errors             []string
	Warnings           []string
	InlinedStylesheets []string
	Stats              Stats
}

// Input is what gets minified: CSS text, a list of stylesheet paths or a
// hash of named sources. Build one with Text, Paths or Sources.
type Input struct {
	kind    inputKind
	text    string
	paths   []string
	sources *jsutil.Object[string]
}

type inputKind int

const (
	textInput inputKind = iota
	pathsInput
	sourcesInput
)

// Text returns an input of CSS text.
func Text(css string) Input {
	return Input{kind: textInput, text: css}
}

// Paths returns an input of stylesheet paths, relative to the working
// directory, read in order.
func Paths(paths ...string) Input {
	return Input{kind: pathsInput, paths: paths}
}

// Sources returns an input of named sources: each key is a path and each
// value the CSS text of that path.
func Sources(sources *jsutil.Object[string]) Input {
	return Input{kind: sourcesInput, sources: sources}
}

// Minify minifies the input into a single output.
func (m *Minifier) Minify(input Input) *Output {
	ctx := &optimizer.Context{
		Options:       m.options,
		Validator:     optimizer.NewValidator(m.options.Compatibility),
		Specificities: map[string][3]int{},
		Warnings:      []string{},
	}

	if m.options.Rebase && !m.options.ExplicitRebaseTo {
		ctx.Warn(implicitRebaseToWarning)
	}

	read := reader.NewContext(m.options, ctx.Validator, &ctx.Warnings)
	var tokens []*token.Token
	switch input.kind {
	case pathsInput:
		tokens = read.ReadPaths(input.paths)
	case sourcesInput:
		tokens = read.ReadHash(input.sources)
	default:
		tokens = read.ReadString(input.text)
	}

	styles := writer.Styles(m.optimize(tokens, ctx), m.options)
	minifiedSize := jsutil.Len16(styles)

	return &Output{
		Styles:             styles,
		Errors:             read.Errors,
		Warnings:           ctx.Warnings,
		InlinedStylesheets: read.InlinedStylesheets,
		Stats: Stats{
			OriginalSize: read.OriginalSize,
			MinifiedSize: minifiedSize,
			Efficiency:   1 - float64(minifiedSize)/float64(read.OriginalSize),
		},
	}
}

func (m *Minifier) optimize(tokens []*token.Token, ctx *optimizer.Context) []*token.Token {
	if m.options.Level.Enabled(options.One) {
		tokens = optimizer.Level1(tokens, ctx)
	}
	if m.options.Level.Enabled(options.Two) {
		tokens = level2.Optimize(tokens, ctx, true)
	}
	return tokens
}

// MinifyBatch minifies every source of a paths or sources input on its own,
// keyed by path in the order of a JavaScript object: a path is minified as a
// one-path input and a named source as CSS text. A text input is minified as
// a whole under the empty key.
func (m *Minifier) MinifyBatch(input Input) *jsutil.Object[*Output] {
	outputs := jsutil.NewObject[*Output]()
	switch input.kind {
	case pathsInput:
		for _, p := range input.paths {
			outputs.Set(p, m.Minify(Paths(p)))
		}
	case sourcesInput:
		for _, key := range input.sources.Keys() {
			styles, _ := input.sources.Get(key)
			outputs.Set(key, m.Minify(Text(styles)))
		}
	default:
		outputs.Set("", m.Minify(input))
	}
	return outputs
}

<h1 align="center">
  <br/>
  <img src="./logo.v2.svg" alt="clean-css logo" width="525px"/>
  <br/>
  <br/>
</h1>

clean-css is a fast and efficient CSS optimizer, written in Go. It is available as a Go library and as the `cleancss` command.

**Table of Contents**

- [Go version support](#go-version-support)
- [Install](#install)
- [Use](#use)
  * [Options](#options)
  * [Compatibility modes](#compatibility-modes)
  * [Formatting options](#formatting-options)
  * [Inlining options](#inlining-options)
  * [Optimization levels](#optimization-levels)
    + [Level 0 optimizations](#level-0-optimizations)
    + [Level 1 optimizations](#level-1-optimizations)
    + [Level 2 optimizations](#level-2-optimizations)
  * [Minify method](#minify-method)
  * [Command line](#command-line)
- [FAQ](#faq)
  * [How to optimize multiple files?](#how-to-optimize-multiple-files)
  * [How to process multiple files without concatenating them into one output file?](#how-to-process-multiple-files-without-concatenating-them-into-one-output-file)
  * [What happens to remote `@import`s?](#what-happens-to-remote-imports)
  * [How to specify a custom rounding precision?](#how-to-specify-a-custom-rounding-precision)
  * [How to optimize a stylesheet with custom `rpx` units?](#how-to-optimize-a-stylesheet-with-custom-rpx-units)
  * [How to keep a CSS fragment intact?](#how-to-keep-a-css-fragment-intact)
  * [How to preserve a comment block?](#how-to-preserve-a-comment-block)
  * [How to rebase relative image URLs?](#how-to-rebase-relative-image-urls)
  * [How to apply level 1 & 2 optimizations at the same time?](#how-to-apply-level-1--2-optimizations-at-the-same-time)
  * [What level 2 optimizations do?](#what-level-2-optimizations-do)
  * [What errors and warnings are?](#what-errors-and-warnings-are)
- [Contributing](#contributing)
  * [How to get started?](#how-to-get-started)
- [Acknowledgments](#acknowledgments)
- [License](#license)

# Go version support

clean-css requires Go 1.23 or newer and uses the standard library only.

# Install

As a library:

```
go get github.com/clean-css/clean-css
```

As a command:

```
go install github.com/clean-css/clean-css/cmd/cleancss@latest
```

# Use

```go
package main

import (
	"fmt"

	cleancss "github.com/clean-css/clean-css"
)

func main() {
	minifier, err := cleancss.New(nil) // nil means default options
	if err != nil {
		panic(err)
	}
	output := minifier.Minify(cleancss.Text("a{font-weight:bold;}"))
	fmt.Println(output.Styles) // a{font-weight:700}
}
```

## Options

`cleancss.New` takes the options as an ordered JSON object (`*jsutil.Object[any]`, which `jsutil.ParseJSON` returns for a JSON document); option values keep the loose typing of clean-css' JavaScript options, so a JSON document of options behaves exactly as the same object did there. The options are:

* `batch` - minifies every source of a list of paths or a hash on its own; defaults to `false`; see [how to process multiple files without concatenating them](#how-to-process-multiple-files-without-concatenating-them-into-one-output-file);
* `compatibility` - controls compatibility mode used; defaults to `ie10+`; see [compatibility modes](#compatibility-modes) for examples;
* `format` - controls output CSS formatting; defaults to `false`; see [formatting options](#formatting-options) for examples;
* `inline` - controls `@import` inlining rules; defaults to `'local'`; see [inlining options](#inlining-options) for examples;
* `level` - controls optimization level used; defaults to `1`; see [optimization levels](#optimization-levels) for examples;
* `rebase` - controls URL rebasing; defaults to `false`;
* `rebaseTo` - controls a directory to which all URLs are rebased, most likely the directory under which the output file will live; defaults to the current directory.

Source maps, fetching remote stylesheets and JavaScript plugins are not part of the Go version.

The option examples below are written in JavaScript object notation with comments, as in the original documentation; pass the same structure as JSON.

## Compatibility modes

There is a certain number of compatibility mode shortcuts, namely:

* `{ compatibility: '*' }` (default) - Internet Explorer 10+ compatibility mode
* `{ compatibility: 'ie9' }` - Internet Explorer 9+ compatibility mode
* `{ compatibility: 'ie8' }` - Internet Explorer 8+ compatibility mode
* `{ compatibility: 'ie7' }` - Internet Explorer 7+ compatibility mode

Each of these modes is an alias to a fine grained configuration (see `internal/options/compatibility.go`), with the following options available:

```jsonc
{
  compatibility: {
    colors: {
      hexAlpha: false, // controls 4- and 8-character hex color support
      opacity: true // controls `rgba()` / `hsla()` color support
    },
    properties: {
      backgroundClipMerging: true, // controls background-clip merging into shorthand
      backgroundOriginMerging: true, // controls background-origin merging into shorthand
      backgroundSizeMerging: true, // controls background-size merging into shorthand
      colors: true, // controls color optimizations
      ieBangHack: false, // controls keeping IE bang hack
      ieFilters: false, // controls keeping IE `filter` / `-ms-filter`
      iePrefixHack: false, // controls keeping IE prefix hack
      ieSuffixHack: false, // controls keeping IE suffix hack
      merging: true, // controls property merging based on understandability
      shorterLengthUnits: false, // controls shortening pixel units into `pc`, `pt`, or `in` units
      spaceAfterClosingBrace: true, // controls keeping space after closing brace - `url() no-repeat` into `url()no-repeat`
      urlQuotes: true, // controls keeping quoting inside `url()`
      zeroUnits: true // controls removal of units `0` value
    },
    selectors: {
      adjacentSpace: false, // controls extra space before `nav` element
      ie7Hack: true, // controls removal of IE7 selector hacks, e.g. `*+html...`
      mergeablePseudoClasses: [':active', ...], // controls a whitelist of mergeable pseudo classes
      mergeablePseudoElements: ['::after', ...], // controls a whitelist of mergeable pseudo elements
      mergeLimit: 8191, // controls maximum number of selectors in a single rule (since 4.1.0)
      multiplePseudoMerging: true // controls merging of rules with multiple pseudo classes / elements (since 4.1.0)
    },
    units: {
      ch: true, // controls treating `ch` as a supported unit
      in: true, // controls treating `in` as a supported unit
      pc: true, // controls treating `pc` as a supported unit
      pt: true, // controls treating `pt` as a supported unit
      rem: true, // controls treating `rem` as a supported unit
      vh: true, // controls treating `vh` as a supported unit
      vm: true, // controls treating `vm` as a supported unit
      vmax: true, // controls treating `vmax` as a supported unit
      vmin: true // controls treating `vmin` as a supported unit
    }
  }
}
```

You can also use a string when setting a compatibility mode, e.g.

```jsonc
{
  compatibility: 'ie9,-properties.merging' // sets compatibility to IE9 mode with disabled property merging
}
```

## Formatting options

By default output CSS is formatted without any whitespace unless a `format` option is given.
First of all there are two shorthands:

```jsonc
{
  format: 'beautify' // formats output in a really nice way
}
```

and

```jsonc
{
  format: 'keep-breaks' // formats output the default way but adds line breaks for improved readability
}
```

however `format` option also accept a fine-grained set of options:

```jsonc
{
  format: {
    breaks: { // controls where to insert breaks
      afterAtRule: false, // controls if a line break comes after an at-rule; e.g. `@charset`; defaults to `false`
      afterBlockBegins: false, // controls if a line break comes after a block begins; e.g. `@media`; defaults to `false`
      afterBlockEnds: false, // controls if a line break comes after a block ends, defaults to `false`
      afterComment: false, // controls if a line break comes after a comment; defaults to `false`
      afterProperty: false, // controls if a line break comes after a property; defaults to `false`
      afterRuleBegins: false, // controls if a line break comes after a rule begins; defaults to `false`
      afterRuleEnds: false, // controls if a line break comes after a rule ends; defaults to `false`
      beforeBlockEnds: false, // controls if a line break comes before a block ends; defaults to `false`
      betweenSelectors: false // controls if a line break comes between selectors; defaults to `false`
    },
    breakWith: '\n', // controls the new line character, can be `'\r\n'` or `'\n'` (aliased as `'windows'` and `'unix'` or `'crlf'` and `'lf'`); defaults to system one, so former on Windows and latter on Unix
    indentBy: 0, // controls number of characters to indent with; defaults to `0`
    indentWith: 'space', // controls a character to indent with, can be `'space'` or `'tab'`; defaults to `'space'`
    spaces: { // controls where to insert spaces
      aroundSelectorRelation: false, // controls if spaces come around selector relations; e.g. `div > a`; defaults to `false`
      beforeBlockBegins: false, // controls if a space comes before a block begins; e.g. `.block {`; defaults to `false`
      beforeValue: false // controls if a space comes before a value; e.g. `width: 1rem`; defaults to `false`
    },
    wrapAt: false, // controls maximum line length; defaults to `false`
    semicolonAfterLastProperty: false // controls removing trailing semicolons in rule; defaults to `false` - means remove
  }
}
```

Also since clean-css 5.0 you can use numerical values for all line breaks, which will repeat a line break that many times, e.g:

```jsonc
  {
    format: {
      breaks: {
        afterAtRule: 2,
        afterBlockBegins: 1, // 1 is synonymous with `true`
        afterBlockEnds: 2,
        afterComment: 1,
        afterProperty: 1,
        afterRuleBegins: 1,
        afterRuleEnds: 1,
        beforeBlockEnds: 1,
        betweenSelectors: 0 // 0 is synonymous with `false`
      }
    }
  }
```

which will add nicer spacing between at rules and blocks.

## Inlining options

`inline` option whitelists which `@import` rules will be processed, e.g.

```jsonc
{
  inline: ['local'] // default; enables local inlining only
}
```

```jsonc
{
  inline: ['none'] // disables all inlining
}
```

```jsonc
// introduced in clean-css 4.1.0
{
  inline: false // disables all inlining (alias to `['none']`)
}
```

```jsonc
{
  inline: ['all'] // enables all inlining, same as ['local', 'remote']
}
```

```jsonc
{
  inline: ['local', 'mydomain.example.com'] // enables local inlining plus given remote source
}
```

```jsonc
{
  inline: ['local', 'remote', '!fonts.googleapis.com'] // enables all inlining but from given remote source
}
```

## Optimization levels

The `level` option can be either `0`, `1` (default), or `2`, e.g.

```jsonc
{
  level: 2
}
```

or a fine-grained configuration given via a hash.

Please note that level 1 optimization options are generally safe while level 2 optimizations should be safe for most users.

### Level 0 optimizations

Level 0 optimizations simply means "no optimizations". Use it when you'd like to inline imports and / or rebase URLs but skip everything else.

### Level 1 optimizations

Level 1 optimizations (default) operate on single properties only, e.g. can remove units when not required, turn rgb colors to a shorter hex representation, remove comments, etc

Here is a full list of available options:

```jsonc
{
  level: {
    1: {
      cleanupCharsets: true, // controls `@charset` moving to the front of a stylesheet; defaults to `true`
      normalizeUrls: true, // controls URL normalization; defaults to `true`
      optimizeBackground: true, // controls `background` property optimizations; defaults to `true`
      optimizeBorderRadius: true, // controls `border-radius` property optimizations; defaults to `true`
      optimizeFilter: true, // controls `filter` property optimizations; defaults to `true`
      optimizeFont: true, // controls `font` property optimizations; defaults to `true`
      optimizeFontWeight: true, // controls `font-weight` property optimizations; defaults to `true`
      optimizeOutline: true, // controls `outline` property optimizations; defaults to `true`
      removeEmpty: true, // controls removing empty rules and nested blocks; defaults to `true`
      removeNegativePaddings: true, // controls removing negative paddings; defaults to `true`
      removeQuotes: true, // controls removing quotes when unnecessary; defaults to `true`
      removeWhitespace: true, // controls removing unused whitespace; defaults to `true`
      replaceMultipleZeros: true, // contols removing redundant zeros; defaults to `true`
      replaceTimeUnits: true, // controls replacing time units with shorter values; defaults to `true`
      replaceZeroUnits: true, // controls replacing zero values with units; defaults to `true`
      roundingPrecision: false, // rounds pixel values to `N` decimal places; `false` disables rounding; defaults to `false`
      selectorsSortingMethod: 'standard', // denotes selector sorting method; can be `'natural'` or `'standard'`, `'none'`, or false (the last two since 4.1.0); defaults to `'standard'`
      specialComments: 'all', // denotes a number of /*! ... */ comments preserved; defaults to `all`
      tidyAtRules: true, // controls at-rules (e.g. `@charset`, `@import`) optimizing; defaults to `true`
      tidyBlockScopes: true, // controls block scopes (e.g. `@media`) optimizing; defaults to `true`
      tidySelectors: true, // controls selectors optimizing; defaults to `true`,
      variableValueOptimizers: [] // controls value optimizers which are applied to variables
    }
  }
}
```

There is an `all` shortcut for toggling all options at the same time, e.g.

```jsonc
{
  level: {
    1: {
      all: false, // set all values to `false`
      tidySelectors: true // turns on optimizing selectors
    }
  }
}
```

### Level 2 optimizations

Level 2 optimizations operate at rules or multiple properties level, e.g. can remove duplicate rules, remove properties redefined further down a stylesheet, or restructure rules by moving them around.

Please note that if level 2 optimizations are turned on then, unless explicitely disabled, level 1 optimizations are applied as well.

Here is a full list of available options:

```jsonc
{
  level: {
    2: {
      mergeAdjacentRules: true, // controls adjacent rules merging; defaults to true
      mergeIntoShorthands: true, // controls merging properties into shorthands; defaults to true
      mergeMedia: true, // controls `@media` merging; defaults to true
      mergeNonAdjacentRules: true, // controls non-adjacent rule merging; defaults to true
      mergeSemantically: false, // controls semantic merging; defaults to false
      overrideProperties: true, // controls property overriding based on understandability; defaults to true
      removeEmpty: true, // controls removing empty rules and nested blocks; defaults to `true`
      reduceNonAdjacentRules: true, // controls non-adjacent rule reducing; defaults to true
      removeDuplicateFontRules: true, // controls duplicate `@font-face` removing; defaults to true
      removeDuplicateMediaBlocks: true, // controls duplicate `@media` removing; defaults to true
      removeDuplicateRules: true, // controls duplicate rules removing; defaults to true
      removeUnusedAtRules: false, // controls unused at rule removing; defaults to false (available since 4.1.0)
      restructureRules: false, // controls rule restructuring; defaults to false
      skipProperties: [] // controls which properties won't be optimized, defaults to `[]` which means all will be optimized (since 4.1.0)
    }
  }
}
```

There is an `all` shortcut for toggling all options at the same time, e.g.

```jsonc
{
  level: {
    2: {
      all: false, // sets all values to `false`
      removeDuplicateRules: true // turns on removing duplicate rules
    }
  }
}
```

## Minify method

Once configured, a `Minifier` optimizes a given input:

```go
output := minifier.Minify(cleancss.Text(source))
```

The output has the following fields:

```go
output.Styles             // optimized output CSS as a string
output.Errors             // a list of errors raised
output.Warnings           // a list of warnings raised
output.InlinedStylesheets // absolute paths of the inlined local stylesheets
output.Stats.OriginalSize // original content size after import inlining, in UTF-16 code units
output.Stats.MinifiedSize // optimized content size, in UTF-16 code units
output.Stats.Efficiency   // 1 - MinifiedSize / OriginalSize, e.g. 0.25 if size is reduced from 100 to 75
```

Example: minifying a CSS string

```go
output := minifier.Minify(cleancss.Text(`
  a {
    color: blue;
  }
  div {
    margin: 5px
  }
`))
fmt.Println(output.Styles) // a{color:#00f}div{margin:5px}
```

To optimize a file without reading it first, pass its path:

```go
output := minifier.Minify(cleancss.Paths("path/to/file.css"))
```

There are several ways to optimize multiple files at the same time, see [How to optimize multiple files?](#how-to-optimize-multiple-files).

## Command line

The `cleancss` command minifies standard input, or the files given as arguments (concatenated in order), and writes the result to standard output:

```
cleancss [flags] [FILE...]
  -O, --options JSON       clean-css options as a JSON object (default {}); repeatable, the last one wins
      --options-file FILE  read the options object from a JSON file ("-" reads standard input)
      --input-hash FILE    minify a hash input: FILE holds a JSON object mapping each source path to
                           an object whose "styles" member is that source's CSS text
      --stats              print the statistics after the warnings and errors
      --                   ends the flags; every later argument is a FILE
```

Warnings and errors go to standard error as `WARNING: <text>` and `ERROR: <text>` lines, followed with `--stats` by the `originalSize`, `minifiedSize`, `efficiency` and `inlinedStylesheets` lines. The exit status is 1 when there are errors, 2 for usage errors and 0 otherwise. In batch mode every source is written as a `== <key>` line, its CSS and a newline, and every standard error line is prefixed with `<key>: `.

```
cleancss -O '{"level":2}' styles.css > styles.min.css
```

# FAQ

## How to optimize multiple files?

It can be done either by passing a list of paths, or, when sources are already available, a hash of sources keyed by path:

```go
minifier.Minify(cleancss.Paths("path/to/file/one", "path/to/file/two"))
```

```go
sources := jsutil.NewObject[string]()
sources.Set("path/to/file/one", "contents of file one")
sources.Set("path/to/file/two", "contents of file two")
minifier.Minify(cleancss.Sources(sources))
```

The order of a hash is the [traversal order of object properties](http://2ality.com/2015/10/property-traversal-order-es6.html) of the JavaScript original: keys that are array indexes come first.

Important note - any `@import` rules already present in the hash will be resolved in memory.

## How to process multiple files without concatenating them into one output file?

With the `batch` option set, `MinifyBatch` returns the stylesheets optimized one by one, keyed by path:

```go
outputs := minifier.MinifyBatch(cleancss.Paths("path/to/file/one", "path/to/file/two"))
one, _ := outputs.Get("path/to/file/one") // all other fields, like errors, warnings, or stats are there too
```

## What happens to remote `@import`s?

Remote stylesheets are never fetched: remote `@import` rules are kept as they are and reported with a warning, unless their content is given as a source of a hash input.

## How to specify a custom rounding precision?

The level 1 `roundingPrecision` optimization option accept a string with per-unit rounding precision settings, e.g.

```jsonc
{
  level: {
    1: {
      roundingPrecision: 'all=3,px=5'
    }
  }
}
```

which sets all units rounding precision to 3 digits except `px` unit precision of 5 digits.

## How to optimize a stylesheet with custom `rpx` units?

Since `rpx` is a non standard unit (see [#1074](https://github.com/clean-css/clean-css/issues/1074)), it will be dropped by default as an invalid value.

However you can treat `rpx` units as regular ones:

```jsonc
{
  compatibility: {
    customUnits: {
      rpx: true
    }
  }
}
```

## How to keep a CSS fragment intact?

Note: available since 4.2.0.

Wrap the CSS fragment in special comments which instruct clean-css to preserve it, e.g.

```css
.block-1 {
  color: red
}
/* clean-css ignore:start */
.block-special {
  color: transparent
}
/* clean-css ignore:end */
.block-2 {
  margin: 0
}
```

Optimizing this CSS will result in the following output:

```css
.block-1{color:red}
.block-special {
  color: transparent
}
.block-2{margin:0}
```

## How to preserve a comment block?

Use the `/*!` notation instead of the standard one `/*`:

```css
/*!
  Important comments included in optimized output.
*/
```

## How to rebase relative image URLs?

clean-css will handle it automatically for you in the following cases:

* when full paths to input files are passed in as options;
* when correct paths are passed in via a hash;
* when `rebaseTo` is used with any of above two.

## How to apply level 1 & 2 optimizations at the same time?

Using the hash configuration specifying both optimization levels, e.g.

```jsonc
{
  level: {
    1: {
      all: true,
      normalizeUrls: false
    },
    2: {
      restructureRules: true
    }
  }
}
```

will apply level 1 optimizations, except url normalization, and default level 2 optimizations with rule restructuring.

## What level 2 optimizations do?

All level 2 optimizations are dispatched in `internal/optimizer/level2/optimize.go`, and this is what they do:

* `recursivelyOptimizeBlocks` - does all the following operations on a nested block, like `@media` or `@keyframe`;
* `recursivelyOptimizeProperties` - optimizes properties in rulesets and flat at-rules, like @font-face, by splitting them into components (e.g. `margin` into `margin-(bottom|left|right|top)`), optimizing, and restoring them back. You may want to use `mergeIntoShorthands` option to control whether you want to turn multiple components into shorthands;
* `removeDuplicates` - gets rid of duplicate rulesets with exactly the same set of properties, e.g. when including a Sass / Less partial twice for no good reason;
* `mergeAdjacent` - merges adjacent rulesets with the same selector or rules;
* `reduceNonAdjacent` - identifies which properties are overridden in same-selector non-adjacent rulesets, and removes them;
* `mergeNonAdjacentBySelector` - identifies same-selector non-adjacent rulesets which can be moved (!) to be merged, requires all intermediate rulesets to not redefine the moved properties, or if redefined to have the same value;
* `mergeNonAdjacentByBody` - same as the one above but for same-selector non-adjacent rulesets;
* `restructure` - tries to reorganize different-selector different-rules rulesets so they take less space, e.g. `.one{padding:0}.two{margin:0}.one{margin-bottom:3px}` into `.two{margin:0}.one{padding:0;margin-bottom:3px}`;
* `removeDuplicateFontAtRules` - removes duplicated `@font-face` rules;
* `removeDuplicateMediaQueries` - removes duplicated `@media` nested blocks;
* `mergeMediaQueries` - merges non-adjacent `@media` at-rules by the same rules as `mergeNonAdjacentBy*` above;

## What errors and warnings are?

If clean-css encounters invalid CSS, it will try to remove the invalid part and continue optimizing the rest of the code. It will make you aware of the problem by generating an error or warning. Although clean-css can work with invalid CSS, it is always recommended that you fix warnings and errors in your CSS.

Example: minifying invalid CSS, resulting in two warnings:

```go
output := minifier.Minify(cleancss.Text(`

  a {
    -notarealproperty-: 5px;
    color:
  }
  div {
    margin: 5px
  }

`))
// output.Styles:   div{margin:5px}
// output.Warnings: Invalid property name '-notarealproperty-' at 4:8. Ignoring.
//                  Empty property 'color' at 5:8. Ignoring.
```

Example: minifying CSS with a missing import, resulting in one error:

```go
output := minifier.Minify(cleancss.Text(`

  @import "idontexist.css";
  a {
    color: blue;
  }

`))
// output.Styles: a{color:#00f}
// output.Errors: Ignoring local @import of "idontexist.css" as resource is missing.
```

# Contributing

See [CONTRIBUTING.md](https://github.com/clean-css/clean-css/blob/master/CONTRIBUTING.md).

## How to get started?

First clone the sources:

```bash
git clone git@github.com:clean-css/clean-css.git
```

then use any of the following commands to verify your copy:

```bash
cd clean-css
go vet ./...  # to check the sources
go test ./... # to run all tests
go build ./cmd/cleancss # to build the command
```

# Acknowledgments

Sorted alphabetically by GitHub handle:

* [@abarre](https://github.com/abarre) (Anthony Barre) for improvements to `@import` processing;
* [@alexlamsl](https://github.com/alexlamsl) (Alex Lam S.L.) for testing early clean-css 4 versions, reporting bugs, and suggesting numerous improvements.
* [@altschuler](https://github.com/altschuler) (Simon Altschuler) for fixing `@import` processing inside comments;
* [@ben-eb](https://github.com/ben-eb) (Ben Briggs) for sharing ideas about CSS optimizations;
* [@davisjam](https://github.com/davisjam) (Jamie Davis) for disclosing ReDOS vulnerabilities;
* [@facelessuser](https://github.com/facelessuser) (Isaac) for pointing out a flaw in clean-css' stateless mode;
* [@grandrath](https://github.com/grandrath) (Martin Grandrath) for improving `minify` method source traversal in ES6;
* [@jmalonzo](https://github.com/jmalonzo) (Jan Michael Alonzo) for a patch removing node.js' old `sys` package;
* [@lukeapage](https://github.com/lukeapage) (Luke Page) for suggestions and testing the source maps feature;
  Plus everyone else involved in [#125](https://github.com/clean-css/clean-css/issues/125) for pushing it forward;
* [@madwizard-thomas](https://github.com/madwizard-thomas) for sharing ideas about `@import` inlining and URL rebasing.
* [@ngyikp](https://github.com/ngyikp) (Ng Yik Phang) for testing early clean-css 4 versions, reporting bugs, and suggesting numerous improvements.
* [@wagenet](https://github.com/wagenet) (Peter Wagenet) for suggesting improvements to `@import` inlining behavior;
* [@venemo](https://github.com/venemo) (Timur Kristóf) for an outstanding contribution of advanced property optimizer for 2.2 release;
* [@vvo](https://github.com/vvo) (Vincent Voyer) for a patch with better empty element regex and for inspiring us to do many performance improvements in 0.4 release;
* [@xhmikosr](https://github.com/xhmikosr) for suggesting new features, like option to remove special comments and strip out URLs quotation, and pointing out numerous improvements like JSHint, media queries, etc.

# License

// Package reader reads the input of a minification: CSS text, a list of
// files or a hash of named sources. It tokenizes them, rebases their URLs and
// inlines local @import rules. Remote stylesheets are never fetched.
package reader

import (
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/tokenizer"
)

const fileProtocol = "file://"

// undefinedSource is the key under which the original files an unnamed
// input: its sources are indexed by name and the input has none.
const undefinedSource = "undefined"

// Context collects what reading produces besides tokens and holds the state
// that the original keeps on its minification context.
type Context struct {
	Options   *options.Options
	Validator *optimizer.Validator
	Warnings  *[]string

	Errors             []string
	InlinedStylesheets []string
	// OriginalSize is the length, in UTF-16 code units, of every source read.
	OriginalSize int

	source         string
	sourcesContent map[string]string
}

// NewContext returns a reading context for one minification.
func NewContext(opts *options.Options, validator *optimizer.Validator, warnings *[]string) *Context {
	return &Context{
		Options:            opts,
		Validator:          validator,
		Warnings:           warnings,
		Errors:             []string{},
		InlinedStylesheets: []string{},
		sourcesContent:     map[string]string{},
	}
}

func (c *Context) warn(message string) {
	*c.Warnings = append(*c.Warnings, message)
}

// ReadString reads CSS text.
func (c *Context) ReadString(input string) []*token.Token {
	c.source = ""
	c.sourcesContent[undefinedSource] = input
	c.OriginalSize += jsutil.Len16(input)
	return c.fromStyles(input, &inlineState{inline: c.Options.Inline})
}

// ReadPaths reads a list of stylesheet files (or remote URLs), each as if
// it were imported in turn.
func (c *Context) ReadPaths(paths []string) []*token.Token {
	var imports strings.Builder
	for _, p := range paths {
		imports.WriteString(restoreAsImport(normalizeURI(p)))
	}
	return c.fromStyles(imports.String(), &inlineState{inline: []string{"all"}})
}

// ReadHash reads named sources, keyed by path, each as if it were imported
// in turn; the sources are taken from the hash instead of the file system.
func (c *Context) ReadHash(sources *jsutil.Object[string]) []*token.Token {
	var imports strings.Builder
	for _, uri := range sources.Keys() {
		styles, _ := sources.Get(uri)
		normalized := normalizeURI(uri)
		imports.WriteString(restoreAsImport(normalized))
		c.sourcesContent[normalized] = styles
	}
	return c.fromStyles(imports.String(), &inlineState{inline: []string{"all"}})
}

func normalizeURI(uri string) string {
	if isRemoteResource(uri) {
		return uri
	}
	absolute := uri
	if !isAbsolute(uri) {
		absolute = resolvePath(uri)
	}
	return normalizePath(relativePath(resolvePath(""), absolute))
}

func restoreAsImport(uri string) string {
	return restoreImport("url("+uri+")", "") + ";"
}

// inlineState is what an inlining pass inherits from the one that imported
// the stylesheet: the inline rules in force and whether it came from a
// remote stylesheet.
type inlineState struct {
	inline   []string
	isRemote bool
}

func (c *Context) fromStyles(styles string, parent *inlineState) []*token.Token {
	var config rebaseConfig
	switch {
	case c.source == "":
		config = rebaseConfig{fromBase: resolvePath(""), toBase: c.Options.RebaseTo}
	case isRemoteResource(c.source):
		config = rebaseConfig{fromBase: c.source, toBase: c.source}
	case isAbsolute(c.source):
		config = rebaseConfig{fromBase: dirname(c.source), toBase: c.Options.RebaseTo}
	default:
		config = rebaseConfig{fromBase: dirname(resolvePath(c.source)), toBase: c.Options.RebaseTo}
	}

	tokens := tokenizer.Tokenize(styles, tokenizer.Context{Source: c.source, Warnings: c.Warnings})
	tokens = rebase(tokens, c.Options.Rebase, c.Validator, config)

	if len(parent.inline) == 1 && parent.inline[0] == "none" {
		return tokens
	}
	return c.inline(tokens, parent)
}

// inliner is the state of inlining the imports of one stylesheet.
type inliner struct {
	inlineState
	afterContent bool
	outputTokens []*token.Token
	sourceTokens []*token.Token
}

func (c *Context) inline(tokens []*token.Token, parent *inlineState) []*token.Token {
	in := &inliner{
		inlineState:  *parent,
		outputTokens: []*token.Token{},
		sourceTokens: tokens,
	}

	for {
		at := -1
		for i, t := range in.sourceTokens {
			if t.Kind == token.AtRule && isImport(t.Value) {
				at = i
				break
			}
			in.outputTokens = append(in.outputTokens, t)
			if t.Kind != token.AtRule && t.Kind != token.Comment {
				in.afterContent = true
			}
		}
		if at == -1 {
			return in.outputTokens
		}
		in.sourceTokens = in.sourceTokens[at:]
		c.inlineStylesheet(in)
	}
}

// keepImport passes the @import being processed through unchanged.
func (in *inliner) keepImport() {
	in.outputTokens = append(in.outputTokens, in.sourceTokens[0])
}

func (c *Context) inlineStylesheet(in *inliner) {
	t := in.sourceTokens[0]
	uri, mediaQuery := extractImportURLAndMedia(t.Value)
	if isRemoteResource(uri) {
		c.inlineRemoteStylesheet(uri, mediaQuery, t.Positions, in)
	} else {
		c.inlineLocalStylesheet(uri, mediaQuery, t.Positions, in)
	}
	in.sourceTokens = in.sourceTokens[1:]
}

// inlineRemoteStylesheet handles a remote @import. Nothing is ever fetched:
// only remote stylesheets given as sources of a hash input are inlined.
func (c *Context) inlineRemoteStylesheet(uri, mediaQuery string, positions []token.Position, in *inliner) {
	isAllowed := isAllowedResource(uri, true, in.inline)
	styles, isLoaded := c.sourcesContent[uri]

	switch {
	case slices.Contains(c.InlinedStylesheets, uri):
		c.warn(`Ignoring remote @import of "` + uri + `" as it has already been imported.`)
	case in.afterContent:
		c.warn(`Ignoring remote @import of "` + uri + `" as no callback given and after other content.`)
	case !hasProtocol(uri):
		c.warn(`Skipping remote @import of "` + uri + `" as no protocol given.`)
		in.keepImport()
	case !isLoaded:
		c.warn(`Skipping remote @import of "` + uri + `" as no callback given.`)
		in.keepImport()
	case !isAllowed && in.afterContent:
		c.warn(`Ignoring remote @import of "` + uri + `" as resource is not allowed and after other content.`)
	case !isAllowed:
		c.warn(`Skipping remote @import of "` + uri + `" as resource is not allowed.`)
		in.keepImport()
	default:
		c.InlinedStylesheets = append(c.InlinedStylesheets, uri)
		in.inline = c.Options.Inline
		in.isRemote = true

		c.source = uri
		c.OriginalSize += jsutil.Len16(styles)

		imported := c.fromStyles(styles, &in.inlineState)
		in.outputTokens = append(in.outputTokens, wrapInMedia(imported, mediaQuery, positions)...)
	}
}

func (c *Context) inlineLocalStylesheet(uri, mediaQuery string, positions []token.Position, in *inliner) {
	protocolLessURI := strings.Replace(uri, fileProtocol, "", 1)
	currentPath := resolvePath("")
	var absoluteURI string
	if isAbsolute(protocolLessURI) {
		absoluteURI = resolvePath(currentPath, protocolLessURI[1:])
	} else {
		absoluteURI = resolvePath(c.Options.RebaseTo, protocolLessURI)
	}
	isAllowed := isAllowedResource(protocolLessURI, false, in.inline)
	normalizedPath := normalizePath(relativePath(currentPath, absoluteURI))
	loadedStyles, isLoaded := c.sourcesContent[normalizedPath]

	switch {
	case slices.Contains(c.InlinedStylesheets, absoluteURI):
		c.warn(`Ignoring local @import of "` + protocolLessURI + `" as it has already been imported.`)
	case isAllowed && !isLoaded && !isFile(absoluteURI):
		c.Errors = append(c.Errors, `Ignoring local @import of "`+protocolLessURI+`" as resource is missing.`)
	case !isAllowed && in.afterContent:
		c.warn(`Ignoring local @import of "` + protocolLessURI + `" as resource is not allowed and after other content.`)
	case in.afterContent:
		c.warn(`Ignoring local @import of "` + protocolLessURI + `" as after other content.`)
	case !isAllowed:
		c.warn(`Skipping local @import of "` + protocolLessURI + `" as resource is not allowed.`)
		in.keepImport()
	default:
		importedStyles := loadedStyles
		if !isLoaded {
			data, err := os.ReadFile(absoluteURI)
			if err != nil {
				c.Errors = append(c.Errors, `Ignoring local @import of "`+protocolLessURI+`" as resource is missing.`)
				return
			}
			importedStyles = DecodeUTF8(data)
		}
		importedStyles = strings.TrimPrefix(importedStyles, "\ufeff")

		c.InlinedStylesheets = append(c.InlinedStylesheets, absoluteURI)
		in.inline = c.Options.Inline

		c.source = normalizedPath
		c.sourcesContent[normalizedPath] = importedStyles
		c.OriginalSize += jsutil.Len16(importedStyles)

		imported := c.fromStyles(importedStyles, &in.inlineState)
		in.outputTokens = append(in.outputTokens, wrapInMedia(imported, mediaQuery, positions)...)
	}
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

func wrapInMedia(tokens []*token.Token, mediaQuery string, positions []token.Position) []*token.Token {
	if mediaQuery == "" {
		return tokens
	}
	scope := &token.Token{Kind: token.NestedBlockScope, Value: "@media " + mediaQuery, Positions: positions}
	return []*token.Token{{Kind: token.NestedBlock, Scopes: []*token.Token{scope}, Body: tokens}}
}

// DecodeUTF8 decodes bytes the way node reads a file as UTF-8: every
// maximal invalid subsequence becomes one U+FFFD and a byte order mark is
// kept.
func DecodeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}

	var b strings.Builder
	needed, seen := 0, 0
	var codePoint rune
	lower, upper := byte(0x80), byte(0xbf)
	for i := 0; i < len(data); i++ {
		c := data[i]
		if needed == 0 {
			switch {
			case c <= 0x7f:
				b.WriteByte(c)
			case c >= 0xc2 && c <= 0xdf:
				needed, codePoint = 1, rune(c&0x1f)
			case c >= 0xe0 && c <= 0xef:
				if c == 0xe0 {
					lower = 0xa0
				} else if c == 0xed {
					upper = 0x9f
				}
				needed, codePoint = 2, rune(c&0xf)
			case c >= 0xf0 && c <= 0xf4:
				if c == 0xf0 {
					lower = 0x90
				} else if c == 0xf4 {
					upper = 0x8f
				}
				needed, codePoint = 3, rune(c&0x7)
			default:
				b.WriteRune(utf8.RuneError)
			}
			continue
		}
		if c < lower || c > upper {
			needed, seen, codePoint = 0, 0, 0
			lower, upper = 0x80, 0xbf
			b.WriteRune(utf8.RuneError)
			i--
			continue
		}
		lower, upper = 0x80, 0xbf
		codePoint = codePoint<<6 | rune(c&0x3f)
		seen++
		if seen == needed {
			b.WriteRune(codePoint)
			needed, seen, codePoint = 0, 0, 0
		}
	}
	if needed != 0 {
		b.WriteRune(utf8.RuneError)
	}
	return b.String()
}

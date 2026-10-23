// Package tokenizer turns CSS source into the token tree of package token. It
// is deliberately tolerant: broken input is tokenized the way the original
// tokenizer does, with the same warnings.
package tokenizer

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/token"
)

type level int

const (
	// levelNone is what popping an empty level stack yields; it matches no level.
	levelNone level = iota - 1
	levelBlock
	levelComment
	levelDoubleQuote
	levelRule
	levelSingleQuote
)

var atRules = []string{"@charset", "@import"}

var blockRules = []string{
	"@-moz-document",
	"@document",
	"@-moz-keyframes",
	"@-ms-keyframes",
	"@-o-keyframes",
	"@-webkit-keyframes",
	"@keyframes",
	"@media",
	"@supports",
	"@container",
	"@layer",
}

var pageMarginBoxes = []string{
	"@bottom-center",
	"@bottom-left",
	"@bottom-left-corner",
	"@bottom-right",
	"@bottom-right-corner",
	"@left-bottom",
	"@left-middle",
	"@left-top",
	"@right-bottom",
	"@right-middle",
	"@right-top",
	"@top-center",
	"@top-left",
	"@top-left-corner",
	"@top-right",
	"@top-right-corner",
}

var extraPageBoxes = []string{
	"@footnote",
	"@footnotes",
	"@left",
	"@page-float-bottom",
	"@page-float-top",
	"@right",
}

var (
	ignoreEndCommentPattern   = regexp.MustCompile(`/\* clean-css ignore:end \*/$`)
	ignoreStartCommentPattern = regexp.MustCompile(`^/\* clean-css ignore:start \*/`)
	repeatPattern             = jsutil.MustCompile(`^\[\s{0,31}\d+\s{0,31}\]$`, "")
	tailBrokenValuePattern    = regexp.MustCompile(`([^}])\}*$`)
	ruleWordSeparatorPattern  = jsutil.MustCompile(`[\s(]`, "")
)

// Context receives the warnings raised while tokenizing and names the source
// being tokenized (empty for unnamed input).
type Context struct {
	Source   string
	Warnings *[]string
}

type position struct {
	line   int
	column int
	index  int
	source string
}

// tokenizer walks the source one UTF-16 code unit at a time, like the
// original; units holds the text of each unit (the second unit of a
// surrogate pair is empty).
type tokenizer struct {
	units    []string
	context  Context
	position position
}

// Tokenize tokenizes source.
func Tokenize(source string, context Context) []*token.Token {
	t := &tokenizer{
		units:    codeUnits(source),
		context:  context,
		position: position{line: 1, source: context.Source},
	}
	return t.intoTokens(levelBlock, false)
}

func codeUnits(source string) []string {
	units := make([]string, 0, len(source))
	for i := 0; i < len(source); {
		if source[i] < utf8.RuneSelf {
			units = append(units, source[i:i+1])
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(source[i:])
		units = append(units, source[i:i+size])
		if r > 0xffff {
			units = append(units, "")
		}
		i += size
	}
	return units
}

func (t *tokenizer) at(i int) string {
	if i < 0 || i >= len(t.units) {
		return ""
	}
	return t.units[i]
}

func (t *tokenizer) warn(message string) {
	*t.context.Warnings = append(*t.context.Warnings, message)
}

func (t *tokenizer) here() token.Position {
	return token.Position{Line: t.position.line, Column: t.position.column, Source: t.position.source}
}

func leaf(kind token.Kind, value string, metadata token.Position) *token.Token {
	return &token.Token{Kind: kind, Value: value, Positions: []token.Position{metadata}}
}

func isAlphanumeric(c string) bool {
	if len(c) != 1 {
		return false
	}
	b := c[0]
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-'
}

// intoTokens tokenizes from the current position until the end of the
// source or, when nested, the brace that closes the current block.
func (t *tokenizer) intoTokens(startLevel level, isNested bool) []*token.Token {
	var allTokens []*token.Token
	newTokens := &allTokens
	var ruleToken *token.Token
	var ruleTokens []*token.Token
	var propertyToken *token.Token
	var metadata token.Position
	var metadatas []token.Position
	lvl := startLevel
	var levels []level
	var buffer []string
	var buffers [][]string
	isBufferEmpty := true
	roundBracketLevel := 0
	wasCommentStart := false
	wasCommentEnd := false
	isEscaped := false
	isPreviousDash := false
	isVariable := false
	isRaw := false
	seekingValue := false
	seekingPropertyBlockClosing := false
	pos := &t.position

	popLevel := func() level {
		if len(levels) == 0 {
			return levelNone
		}
		l := levels[len(levels)-1]
		levels = levels[:len(levels)-1]
		return l
	}
	popMetadata := func() token.Position {
		if len(metadatas) == 0 {
			return token.Position{}
		}
		m := metadatas[len(metadatas)-1]
		metadatas = metadatas[:len(metadatas)-1]
		return m
	}
	popBuffer := func() []string {
		if len(buffers) == 0 {
			return nil
		}
		b := buffers[len(buffers)-1]
		buffers = buffers[:len(buffers)-1]
		return b
	}
	popRuleToken := func() *token.Token {
		r := ruleTokens[len(ruleTokens)-1]
		ruleTokens = ruleTokens[:len(ruleTokens)-1]
		return r
	}
	joined := func() string { return strings.Join(buffer, "") }
	pushValue := func(value string) {
		propertyToken.Values = append(propertyToken.Values, leaf(token.PropertyValue, value, metadata))
	}

	for ; pos.index < len(t.units); pos.index++ {
		character := t.units[pos.index]

		isQuoted := lvl == levelSingleQuote || lvl == levelDoubleQuote
		isSpace := character == " " || character == "\t"
		isNewLineNix := character == "\n"
		isNewLineWin := character == "\n" && t.at(pos.index-1) == "\r"
		isCarriageReturn := character == "\r" && t.at(pos.index+1) != "" && t.at(pos.index+1) != "\n"
		isCommentStart := !wasCommentEnd && lvl != levelComment && !isQuoted &&
			character == "*" && t.at(pos.index-1) == "/"
		isCommentEndMarker := !wasCommentStart && !isQuoted && character == "/" && t.at(pos.index-1) == "*"
		isCommentEnd := lvl == levelComment && isCommentEndMarker
		characterWithNoSpecialMeaning := !isSpace && !isCarriageReturn && isAlphanumeric(character)
		isVariable = isVariable || (lvl != levelComment && !seekingValue && isPreviousDash && character == "-" && len(buffer) == 1)
		isPreviousDash = character == "-"
		if roundBracketLevel < 0 {
			roundBracketLevel = 0
		}

		if isBufferEmpty {
			metadata = t.here()
		}

		switch {
		case isEscaped:
			// previous character was a backslash
			buffer = append(buffer, character)
			isBufferEmpty = false
		case characterWithNoSpecialMeaning:
			// an alphanumeric character or a hyphen (part of any rule or property name)
			buffer = append(buffer, character)
			isBufferEmpty = false
		case (isSpace || isNewLineNix && !isNewLineWin) && (isQuoted || lvl == levelComment):
			buffer = append(buffer, character)
			isBufferEmpty = false
		case (isSpace || isNewLineNix && !isNewLineWin) && isBufferEmpty:
			// noop
		case !isCommentEnd && lvl == levelComment:
			buffer = append(buffer, character)
			isBufferEmpty = false
		case !isCommentStart && !isCommentEnd && isRaw:
			buffer = append(buffer, character)
			isBufferEmpty = false
		case isCommentStart && isVariable && (lvl == levelBlock || lvl == levelRule) && len(buffer) > 1:
			// comment start within a variable, e.g. var(/*<--
			buffer = append(buffer, character)
			isBufferEmpty = false
			levels = append(levels, lvl)
			lvl = levelComment
		case isCommentStart && (lvl == levelBlock || lvl == levelRule) && len(buffer) > 1:
			// comment start within block preceded by some content, e.g. div/*<--
			metadatas = append(metadatas, metadata)
			buffer = append(buffer, character)
			buffers = append(buffers, slices.Clone(buffer[:len(buffer)-2]))
			isBufferEmpty = false
			buffer = slices.Clone(buffer[len(buffer)-2:])
			metadata = token.Position{Line: pos.line, Column: pos.column - 1, Source: pos.source}
			levels = append(levels, lvl)
			lvl = levelComment
		case isCommentStart:
			// comment start, e.g. /*<--
			levels = append(levels, lvl)
			lvl = levelComment
			buffer = append(buffer, character)
			isBufferEmpty = false
		case isCommentEnd && isVariable:
			// comment end within a variable, e.g. var(/*!*/<--
			buffer = append(buffer, character)
			lvl = popLevel()
		case isCommentEnd && ignoreStartCommentPattern.MatchString(joined()+"/"):
			// ignore:start comment end, e.g. /* clean-css ignore:start */<--
			serialized := jsutil.Trim(joined()) + character
			*newTokens = append(*newTokens, leaf(token.Comment, serialized, metadata))
			isRaw = true
			metadata = popMetadata()
			buffer = popBuffer()
			isBufferEmpty = len(buffer) == 0
		case isCommentEnd && ignoreEndCommentPattern.MatchString(joined()+"/"):
			// ignore:end comment end, e.g. /* clean-css ignore:end */<--
			serialized := joined() + character
			lastCommentStartAt := strings.LastIndex(serialized, "/*")
			raw := serialized[:lastCommentStartAt]
			*newTokens = append(*newTokens, leaf(token.Raw, raw, metadata))
			comment := serialized[lastCommentStartAt:]
			metadata = token.Position{Line: pos.line, Column: pos.column - jsutil.Len16(comment) + 1, Source: pos.source}
			*newTokens = append(*newTokens, leaf(token.Comment, comment, metadata))
			isRaw = false
			lvl = popLevel()
			metadata = popMetadata()
			buffer = popBuffer()
			isBufferEmpty = len(buffer) == 0
		case isCommentEnd:
			// comment end, e.g. /* comment */<--
			serialized := jsutil.Trim(joined()) + character
			*newTokens = append(*newTokens, leaf(token.Comment, serialized, metadata))
			lvl = popLevel()
			metadata = popMetadata()
			buffer = popBuffer()
			isBufferEmpty = len(buffer) == 0
		case isCommentEndMarker && t.at(pos.index+1) != "*":
			t.warn("Unexpected '*/' at " + t.here().String() + ".")
			buffer = nil
			isBufferEmpty = true
		case character == "'" && !isQuoted:
			// single quotation start, e.g. a[href^='https<--
			levels = append(levels, lvl)
			lvl = levelSingleQuote
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character == "'" && lvl == levelSingleQuote:
			// single quotation end, e.g. a[href^='https'<--
			lvl = popLevel()
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character == `"` && !isQuoted:
			// double quotation start, e.g. a[href^="<--
			levels = append(levels, lvl)
			lvl = levelDoubleQuote
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character == `"` && lvl == levelDoubleQuote:
			// double quotation end, e.g. a[href^="https"<--
			lvl = popLevel()
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character != ")" && character != "(" && lvl != levelComment && !isQuoted && roundBracketLevel > 0:
			// character inside any function, e.g. hsla(.<--
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character == "(" && !isQuoted && lvl != levelComment && !seekingValue:
			// round open bracket, e.g. @import url(<--
			buffer = append(buffer, character)
			isBufferEmpty = false
			roundBracketLevel++
		case character == ")" && !isQuoted && lvl != levelComment && !seekingValue:
			// round close bracket, e.g. @import url(test.css)<--
			buffer = append(buffer, character)
			isBufferEmpty = false
			roundBracketLevel--
		case character == ";" && lvl == levelBlock && len(buffer) > 0 && buffer[0] == "@":
			// semicolon ending rule at block level, e.g. @import '...';<--
			serialized := jsutil.Trim(joined())
			allTokens = append(allTokens, leaf(token.AtRule, serialized, metadata))
			buffer = nil
			isBufferEmpty = true
		case character == "," && lvl == levelBlock && ruleToken != nil:
			// comma separator at block level, e.g. a,div,<--
			serialized := jsutil.Trim(joined())
			ruleToken.Scopes = append(ruleToken.Scopes, leaf(scopeKind(ruleToken.Kind), serialized, metadata))
			buffer = nil
			isBufferEmpty = true
		case character == "," && lvl == levelBlock && tokenKindFrom(buffer) == token.AtRule:
			// comma separator at block level, e.g. @import url(...) screen,<--
			// keep iterating as end semicolon will create the token
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character == "," && lvl == levelBlock:
			// comma separator at block level, e.g. a,<--
			ruleToken = &token.Token{Kind: tokenKindFrom(buffer)}
			serialized := jsutil.Trim(joined())
			ruleToken.Scopes = append(ruleToken.Scopes, leaf(scopeKind(ruleToken.Kind), serialized, metadata))
			buffer = nil
			isBufferEmpty = true
		case character == "{" && lvl == levelBlock && ruleToken != nil && ruleToken.Kind == token.NestedBlock:
			// open brace opening at-rule at block level, e.g. @media{<--
			serialized := jsutil.Trim(joined())
			ruleToken.Scopes = append(ruleToken.Scopes, leaf(token.NestedBlockScope, serialized, metadata))
			allTokens = append(allTokens, ruleToken)
			levels = append(levels, lvl)
			pos.column++
			pos.index++
			buffer = nil
			isBufferEmpty = true
			ruleToken.Body = t.intoTokens(levelBlock, true)
			ruleToken = nil
		case character == "{" && lvl == levelBlock && tokenKindFrom(buffer) == token.NestedBlock:
			// open brace opening at-rule at block level, e.g. @media{<--
			serialized := jsutil.Trim(joined())
			if ruleToken == nil {
				ruleToken = &token.Token{Kind: token.NestedBlock}
			}
			ruleToken.Scopes = append(ruleToken.Scopes, leaf(token.NestedBlockScope, serialized, metadata))
			allTokens = append(allTokens, ruleToken)
			levels = append(levels, lvl)
			pos.column++
			pos.index++
			buffer = nil
			isBufferEmpty = true
			isVariable = false
			ruleToken.Body = t.intoTokens(levelBlock, true)
			ruleToken = nil
		case character == "{" && lvl == levelBlock:
			// open brace opening rule at block level, e.g. div{<--
			serialized := jsutil.Trim(joined())
			if ruleToken == nil {
				ruleToken = &token.Token{Kind: tokenKindFrom(buffer)}
			}
			ruleToken.Scopes = append(ruleToken.Scopes, leaf(scopeKind(ruleToken.Kind), serialized, metadata))
			ruleToken.Body = []*token.Token{}
			newTokens = &ruleToken.Body
			allTokens = append(allTokens, ruleToken)
			levels = append(levels, lvl)
			lvl = levelRule
			buffer = nil
			isBufferEmpty = true
		case character == "{" && lvl == levelRule && seekingValue:
			// open brace opening rule at rule level, e.g. div{--variable:{<--
			ruleTokens = append(ruleTokens, ruleToken)
			ruleToken = &token.Token{Kind: token.PropertyBlock, Body: []*token.Token{}}
			propertyToken.Values = append(propertyToken.Values, ruleToken)
			newTokens = &ruleToken.Body
			levels = append(levels, lvl)
			lvl = levelRule
			seekingValue = false
		case character == "{" && lvl == levelRule && isPageMarginBox(buffer):
			// open brace opening page-margin box at rule level, e.g. @page{@top-center{<--
			serialized := jsutil.Trim(joined())
			ruleTokens = append(ruleTokens, ruleToken)
			ruleToken = &token.Token{Kind: token.AtRuleBlock, Body: []*token.Token{}}
			ruleToken.Scopes = append(ruleToken.Scopes, leaf(token.AtRuleBlockScope, serialized, metadata))
			*newTokens = append(*newTokens, ruleToken)
			newTokens = &ruleToken.Body
			levels = append(levels, lvl)
			lvl = levelRule
			buffer = nil
			isBufferEmpty = true
		case character == ":" && lvl == levelRule && !seekingValue:
			// colon at rule level, e.g. a{color:<--
			serialized := jsutil.Trim(joined())
			propertyToken = &token.Token{Kind: token.Property, Name: leaf(token.PropertyName, serialized, metadata)}
			*newTokens = append(*newTokens, propertyToken)
			seekingValue = true
			buffer = nil
			isBufferEmpty = true
		case character == ";" && lvl == levelRule && propertyToken != nil && len(ruleTokens) > 0 && !isBufferEmpty && buffer[0] == "@":
			// semicolon at rule level for at-rule, e.g. a{--color:{@apply(--other-color);<--
			serialized := jsutil.Trim(joined())
			ruleToken.Body = append(ruleToken.Body, leaf(token.AtRule, serialized, metadata))
			buffer = nil
			isBufferEmpty = true
		case character == ";" && lvl == levelRule && propertyToken != nil && !isBufferEmpty:
			// semicolon at rule level, e.g. a{color:red;<--
			pushValue(jsutil.Trim(joined()))
			propertyToken = nil
			seekingValue = false
			buffer = nil
			isBufferEmpty = true
			isVariable = false
		case character == ";" && lvl == levelRule && propertyToken != nil && isBufferEmpty && isVariable && len(propertyToken.Values) == 0:
			// semicolon after empty variable value at rule level, e.g. a{--color: ;<--
			pushValue(" ")
			isVariable = false
			propertyToken = nil
			seekingValue = false
		case character == ";" && lvl == levelRule && propertyToken != nil && isBufferEmpty:
			// semicolon after bracketed value at rule level, e.g. a{color:rgb(...);<--
			propertyToken = nil
			seekingValue = false
		case character == ";" && lvl == levelRule && !isBufferEmpty && buffer[0] == "@":
			// semicolon for at-rule at rule level, e.g. a{@apply(--variable);<--
			*newTokens = append(*newTokens, leaf(token.AtRule, joined(), metadata))
			seekingValue = false
			buffer = nil
			isBufferEmpty = true
		case character == ";" && lvl == levelRule && seekingPropertyBlockClosing:
			// close brace after a property block at rule level, e.g. a{--custom:{color:red;};<--
			seekingPropertyBlockClosing = false
			buffer = nil
			isBufferEmpty = true
		case character == ";" && lvl == levelRule && isBufferEmpty:
			// stray semicolon at rule level, e.g. a{;<--
		case character == "}" && lvl == levelRule && propertyToken != nil && seekingValue && !isBufferEmpty && len(ruleTokens) > 0:
			// close brace at rule level, e.g. a{--color:{color:red}<--
			pushValue(joined())
			propertyToken = nil
			ruleToken = popRuleToken()
			newTokens = &ruleToken.Body
			lvl = popLevel()
			seekingValue = false
			buffer = nil
			isBufferEmpty = true
		case character == "}" && lvl == levelRule && propertyToken != nil && !isBufferEmpty && buffer[0] == "@" && len(ruleTokens) > 0:
			// close brace at rule level for at-rule, e.g. a{--color:{@apply(--other-color)}<--
			ruleToken.Body = append(ruleToken.Body, leaf(token.AtRule, joined(), metadata))
			propertyToken = nil
			ruleToken = popRuleToken()
			newTokens = &ruleToken.Body
			lvl = popLevel()
			seekingValue = false
			buffer = nil
			isBufferEmpty = true
		case character == "}" && lvl == levelRule && propertyToken != nil && len(ruleTokens) > 0:
			// close brace at rule level after space, e.g. a{--color:{color:red }<--
			propertyToken = nil
			ruleToken = popRuleToken()
			newTokens = &ruleToken.Body
			lvl = popLevel()
			seekingValue = false
		case character == "}" && lvl == levelRule && propertyToken != nil && !isBufferEmpty:
			// close brace at rule level, e.g. a{color:red}<--
			pushValue(joined())
			propertyToken = nil
			if len(ruleTokens) > 0 {
				ruleToken = popRuleToken()
			} else {
				ruleToken = nil
			}
			newTokens = &allTokens
			lvl = popLevel()
			seekingValue = false
			buffer = nil
			isBufferEmpty = true
		case character == "}" && lvl == levelRule && !isBufferEmpty && buffer[0] == "@":
			// close brace after at-rule at rule level, e.g. a{@apply(--variable)}<--
			propertyToken = nil
			ruleToken = nil
			*newTokens = append(*newTokens, leaf(token.AtRule, jsutil.Trim(joined()), metadata))
			newTokens = &allTokens
			lvl = popLevel()
			seekingValue = false
			buffer = nil
			isBufferEmpty = true
		case character == "}" && lvl == levelRule && len(levels) > 0 && levels[len(levels)-1] == levelRule:
			// close brace after a property block at rule level, e.g. a{--custom:{color:red;}<--
			propertyToken = nil
			ruleToken = popRuleToken()
			newTokens = &ruleToken.Body
			lvl = popLevel()
			seekingValue = false
			seekingPropertyBlockClosing = true
			buffer = nil
			isBufferEmpty = true
		case character == "}" && lvl == levelRule && isVariable && propertyToken != nil && len(propertyToken.Values) == 0:
			// close brace after an empty variable declaration inside a rule, e.g. a{--color: }<--
			pushValue(" ")
			isVariable = false
			propertyToken = nil
			ruleToken = nil
			newTokens = &allTokens
			lvl = popLevel()
			seekingValue = false
			isVariable = false
		case character == "}" && lvl == levelRule:
			// close brace after a rule, e.g. a{color:red;}<--
			propertyToken = nil
			ruleToken = nil
			newTokens = &allTokens
			lvl = popLevel()
			seekingValue = false
			isVariable = false
		case character == "}" && lvl == levelBlock && !isNested && pos.index <= len(t.units)-1:
			// stray close brace at block level, e.g. a{color:red}color:blue}<--
			t.warn("Unexpected '}' at " + t.here().String() + ".")
			buffer = append(buffer, character)
			isBufferEmpty = false
		case character == "}" && lvl == levelBlock:
			// close brace at block level, e.g. @media screen {...}<--
			return t.finish(allTokens, propertyToken, buffer, metadata, seekingValue)
		case character == "(" && lvl == levelRule && seekingValue:
			// round open bracket, e.g. a{color:hsla(<--
			buffer = append(buffer, character)
			isBufferEmpty = false
			roundBracketLevel++
		case character == ")" && lvl == levelRule && seekingValue && roundBracketLevel == 1:
			// round close bracket, e.g. a{color:hsla(0,0%,0%)<--
			buffer = append(buffer, character)
			isBufferEmpty = false
			pushValue(jsutil.Trim(joined()))
			roundBracketLevel--
			buffer = nil
			isBufferEmpty = true
			isVariable = false
		case character == ")" && lvl == levelRule && seekingValue:
			// round close bracket within other brackets, e.g. a{width:calc((10rem / 2)<--
			buffer = append(buffer, character)
			isBufferEmpty = false
			isVariable = false
			roundBracketLevel--
		case character == "/" && t.at(pos.index+1) != "*" && lvl == levelRule && seekingValue && !isBufferEmpty:
			// forward slash within a property, e.g. a{background:url(image.png) 0 0/<--
			pushValue(jsutil.Trim(joined()))
			propertyToken.Values = append(propertyToken.Values, leaf(token.PropertyValue, character, t.here()))
			buffer = nil
			isBufferEmpty = true
		case character == "/" && t.at(pos.index+1) != "*" && lvl == levelRule && seekingValue:
			// forward slash within a property after space, e.g. a{background:url(image.png) 0 0 /<--
			propertyToken.Values = append(propertyToken.Values, leaf(token.PropertyValue, character, t.here()))
			buffer = nil
			isBufferEmpty = true
		case character == "," && lvl == levelRule && seekingValue && !isBufferEmpty:
			// comma within a property, e.g. a{background:url(image.png),<--
			pushValue(jsutil.Trim(joined()))
			propertyToken.Values = append(propertyToken.Values, leaf(token.PropertyValue, character, t.here()))
			buffer = nil
			isBufferEmpty = true
		case character == "," && lvl == levelRule && seekingValue:
			// comma within a property after space, e.g. a{background:url(image.png) ,<--
			propertyToken.Values = append(propertyToken.Values, leaf(token.PropertyValue, character, t.here()))
			buffer = nil
			isBufferEmpty = true
		case character == "]" && propertyToken != nil && !isBufferEmpty && repeatPattern.MatchString(joined()+"]"):
			buffer = append(buffer, character)
			serialized := jsutil.Trim(joined())
			last := propertyToken.Name
			if len(propertyToken.Values) > 0 {
				last = propertyToken.Values[len(propertyToken.Values)-1]
			}
			last.Value += serialized
			buffer = nil
			isBufferEmpty = true
		case (isSpace || isNewLineNix && !isNewLineWin) && lvl == levelRule && seekingValue && propertyToken != nil && !isBufferEmpty:
			// space or *nix newline within property, e.g. a{margin:0 <--
			pushValue(jsutil.Trim(joined()))
			buffer = nil
			isBufferEmpty = true
		case isNewLineWin && lvl == levelRule && seekingValue && propertyToken != nil && len(buffer) > 1:
			// win newline within property, e.g. a{margin:0\r\n<--
			pushValue(jsutil.Trim(joined()))
			buffer = nil
			isBufferEmpty = true
		case isNewLineWin && lvl == levelRule && seekingValue:
			// win newline
			buffer = nil
			isBufferEmpty = true
		case isNewLineWin && len(buffer) == 1:
			// ignore windows newline which is composed of two characters
			buffer = buffer[:0]
			isBufferEmpty = true
		case !isBufferEmpty || !isSpace && !isNewLineNix && !isNewLineWin && !isCarriageReturn:
			// any character
			buffer = append(buffer, character)
			isBufferEmpty = false
		}

		wasEscaped := isEscaped
		isEscaped = !wasEscaped && character == "\\"
		wasCommentStart = isCommentStart
		wasCommentEnd = isCommentEnd

		if isNewLineWin || isNewLineNix || isCarriageReturn {
			pos.line++
			pos.column = 0
		} else {
			pos.column++
		}
	}

	return t.finish(allTokens, propertyToken, buffer, metadata, seekingValue)
}

// finish reports what is left over when a block ends and returns its tokens.
func (t *tokenizer) finish(allTokens []*token.Token, propertyToken *token.Token, buffer []string, metadata token.Position, seekingValue bool) []*token.Token {
	if seekingValue {
		t.warn("Missing '}' at " + t.here().String() + ".")
	}

	if seekingValue && len(buffer) > 0 {
		serialized := jsutil.TrimEnd(strings.Join(buffer, ""))
		serialized = jsutil.TrimEnd(jsutil.Replace(tailBrokenValuePattern, serialized, "$1", false))
		propertyToken.Values = append(propertyToken.Values, leaf(token.PropertyValue, serialized, metadata))
		buffer = nil
	}

	if len(buffer) > 0 {
		t.warn("Invalid character(s) '" + strings.Join(buffer, "") + "' at " + metadata.String() + ". Ignoring.")
	}

	if allTokens == nil {
		allTokens = []*token.Token{}
	}
	return allTokens
}

func tokenKindFrom(buffer []string) token.Kind {
	isAtRule := len(buffer) > 0 && (buffer[0] == "@" || buffer[0] == "_")
	ruleWord := jsutil.Split(ruleWordSeparatorPattern, strings.Join(buffer, ""))[0]

	switch {
	case isAtRule && slices.Contains(blockRules, ruleWord):
		return token.NestedBlock
	case isAtRule && slices.Contains(atRules, ruleWord):
		return token.AtRule
	case isAtRule:
		return token.AtRuleBlock
	}
	return token.Rule
}

func scopeKind(kind token.Kind) token.Kind {
	switch kind {
	case token.Rule:
		return token.RuleScope
	case token.NestedBlock:
		return token.NestedBlockScope
	case token.AtRuleBlock:
		return token.AtRuleBlockScope
	}
	return 0
}

func isPageMarginBox(buffer []string) bool {
	serialized := jsutil.Trim(strings.Join(buffer, ""))
	return slices.Contains(pageMarginBoxes, serialized) || slices.Contains(extraPageBoxes, serialized)
}

package reader

import (
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
)

var (
	protocolLessPrefixPattern = regexp.MustCompile(`^[^\w\d]*//`)
	quotePrefixPattern        = regexp.MustCompile(`^["']`)
	quoteSuffixPattern        = regexp.MustCompile(`["']$`)
	roundBracketsPattern      = regexp.MustCompile(`[()]`)
	urlPrefixPattern          = regexp.MustCompile(`(?i)^url\(`)
	urlSuffixPattern          = regexp.MustCompile(`\)$`)
	whitespacePattern         = jsutil.MustCompile(`\s`, "")
	internalPattern           = regexp.MustCompile(`^\w+:\w+`)
	remotePattern             = regexp.MustCompile(`^[^:]+?://`)
	sourceMapCommentPattern   = jsutil.MustCompile(`^/\*# sourceMappingURL=(\S+) \*/$`, "")
)

// rebaseConfig says which directory URLs are written relative to (fromBase)
// and which directory they are rewritten for (toBase).
type rebaseConfig struct {
	fromBase string
	toBase   string
}

func isRemote(uri string) bool {
	return remotePattern.MatchString(uri) || protocolLessPrefixPattern.MatchString(uri)
}

// rebaseURI rewrites one URL for the target directory.
func rebaseURI(uri string, config rebaseConfig) string {
	switch {
	case isAbsolute(uri) && !isRemote(config.toBase):
		return uri
	case isRemote(uri) || strings.HasPrefix(uri, "#") || internalPattern.MatchString(uri) || isDataURIResource(uri):
		return uri
	case isRemote(config.toBase):
		return resolveURL(config.toBase, uri)
	}
	return relativePath(config.toBase, joinPath(config.fromBase, uri))
}

func quoteFor(unquoted string) string {
	switch {
	case strings.Contains(unquoted, "'"):
		return `"`
	case strings.Contains(unquoted, `"`):
		return "'"
	case whitespacePattern.MatchString(unquoted) || roundBracketsPattern.MatchString(unquoted):
		return "'"
	}
	return ""
}

func stripURL(original string) (stripped, unquoted string) {
	stripped = urlPrefixPattern.ReplaceAllLiteralString(original, "")
	stripped = jsutil.Trim(urlSuffixPattern.ReplaceAllLiteralString(stripped, ""))
	unquoted = quotePrefixPattern.ReplaceAllLiteralString(stripped, "")
	unquoted = jsutil.Trim(quoteSuffixPattern.ReplaceAllLiteralString(unquoted, ""))
	return stripped, unquoted
}

// rewriteURL rebases a url(...) value, keeping or adding the quotes it needs.
func rewriteURL(original string, config rebaseConfig) string {
	stripped, unquoted := stripURL(original)
	quote := quoteFor(unquoted)
	if strings.HasPrefix(stripped, "'") || strings.HasPrefix(stripped, `"`) {
		quote = stripped[:1]
	}
	return "url(" + quote + rebaseURI(unquoted, config) + quote + ")"
}

// rewritePath rebases a bare path, such as the target of a source map comment.
func rewritePath(original string, config rebaseConfig) string {
	_, unquoted := stripURL(original)
	return rebaseURI(unquoted, config)
}

// rebase rewrites the URLs of @import rules and, when rebaseAll is set, of
// every url() value and source map comment.
func rebase(tokens []*token.Token, rebaseAll bool, validator *optimizer.Validator, config rebaseConfig) []*token.Token {
	if rebaseAll {
		rebaseEverything(tokens, validator, config)
	} else {
		for _, t := range tokens {
			if t.Kind == token.AtRule {
				rebaseAtRule(t, config)
			}
		}
	}
	return tokens
}

func rebaseEverything(tokens []*token.Token, validator *optimizer.Validator, config rebaseConfig) {
	for _, t := range tokens {
		switch t.Kind {
		case token.AtRule:
			rebaseAtRule(t, config)
		case token.AtRuleBlock, token.Rule:
			rebaseProperties(t.Body, validator, config)
		case token.Comment:
			rebaseSourceMapComment(t, config)
		case token.NestedBlock:
			rebaseEverything(t.Body, validator, config)
		}
	}
}

func rebaseAtRule(t *token.Token, config rebaseConfig) {
	if !isImport(t.Value) {
		return
	}
	uri, mediaQuery := extractImportURLAndMedia(t.Value)
	t.Value = restoreImport(rewriteURL(uri, config), mediaQuery)
}

func rebaseSourceMapComment(t *token.Token, config rebaseConfig) {
	match, ok := jsutil.Exec(sourceMapCommentPattern, t.Value)
	if ok && !strings.Contains(match.Groups[1], "data:") {
		t.Value = jsutil.ReplaceFirst(t.Value, match.Groups[1], rewritePath(match.Groups[1], config))
	}
}

func rebaseProperties(properties []*token.Token, validator *optimizer.Validator, config rebaseConfig) {
	for _, property := range properties {
		if property.Kind != token.Property {
			continue
		}
		for _, value := range property.Values {
			if value.Kind == token.PropertyValue && validator.IsUrl(value.Value) {
				value.Value = rewriteURL(value.Value, config)
			}
		}
	}
}

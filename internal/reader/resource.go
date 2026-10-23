package reader

import (
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/strutil"
)

var (
	importPrefixPattern     = regexp.MustCompile(`(?i)^@import`)
	remoteResourcePattern   = regexp.MustCompile(`^(\w+://|//)`)
	fileResourcePattern     = regexp.MustCompile(`^file://`)
	noProtocolPattern       = regexp.MustCompile(`^//`)
	dataURIPattern          = jsutil.MustCompile(`^data:(\S{0,31}?)?(;charset=[^;]+)?(;[^,]+?)?,(.+)`, "")
	importURLPrefixPattern  = jsutil.MustCompile(`^url\(\s{0,31}`, "i")
	importURLSuffixPattern  = jsutil.MustCompile(`\s{0,31}\)`, "i")
	importQuotePrefix       = jsutil.MustCompile(`['"]\s{0,31}`, "")
	importQuoteSuffix       = jsutil.MustCompile(`\s{0,31}['"]`, "")
	bracePrefixPattern      = regexp.MustCompile(`^\(`)
	braceSuffixPattern      = regexp.MustCompile(`\)$`)
	windowsSeparatorPattern = regexp.MustCompile(`\\`)
)

// isImport reports whether an at-rule is an @import.
func isImport(value string) bool {
	return importPrefixPattern.MatchString(value)
}

// isRemoteResource reports whether uri points to a remote resource
// (a protocol other than file:// or a protocol-relative URL).
func isRemoteResource(uri string) bool {
	return remoteResourcePattern.MatchString(uri) && !fileResourcePattern.MatchString(uri)
}

// hasProtocol reports whether uri is not protocol-relative.
func hasProtocol(uri string) bool {
	return !noProtocolPattern.MatchString(uri)
}

// isDataURIResource reports whether uri is a data: URI. The look-ahead of
// the original pattern can never fail before a character that is not a
// semicolon, so the charset part reduces to a run of such characters.
func isDataURIResource(uri string) bool {
	return dataURIPattern.MatchString(uri)
}

// normalizePath turns Windows separators into forward slashes.
func normalizePath(p string) string {
	return windowsSeparatorPattern.ReplaceAllLiteralString(p, "/")
}

// restoreImport builds an @import at-rule from a URL and a media query.
func restoreImport(uri, mediaQuery string) string {
	return jsutil.Trim("@import " + uri + " " + mediaQuery)
}

// extractImportURLAndMedia splits an @import at-rule into its URL and media query.
func extractImportURLAndMedia(atRuleValue string) (string, string) {
	normalized := jsutil.Trim(importPrefixPattern.ReplaceAllLiteralString(atRuleValue, ""))
	normalized = jsutil.Replace(importURLPrefixPattern, normalized, "(", false)
	normalized = jsutil.Replace(importURLSuffixPattern, normalized, ") ", false)
	normalized = jsutil.Replace(importQuotePrefix, normalized, "", false)
	normalized = jsutil.Replace(importQuoteSuffix, normalized, "", false)

	parts := strutil.Split(normalized, " ")
	uri := bracePrefixPattern.ReplaceAllLiteralString(parts[0], "")
	uri = braceSuffixPattern.ReplaceAllLiteralString(uri, "")
	return uri, strings.Join(parts[1:], " ")
}

// isAllowedResource reports whether the inline rules allow importing uri.
func isAllowedResource(uri string, isRemote bool, rules []string) bool {
	allowed := !isRemote

	if len(rules) == 0 {
		return false
	}

	if isRemote && !hasProtocol(uri) {
		uri = "http:" + uri
	}

	match := uri
	absoluteURI := uri
	if isRemote {
		match = urlHost(uri)
	} else {
		absoluteURI = resolvePath(uri)
	}

	for _, rule := range rules {
		isNegated := strings.HasPrefix(rule, "!")
		normalizedRule := ""
		if rule != "" {
			normalizedRule = rule[1:]
		}

		switch {
		case isNegated && isRemote && isRemoteRule(normalizedRule):
			allowed = allowed && !isAllowedResource(uri, true, []string{normalizedRule})
		case isNegated && !isRemote && !isRemoteRule(normalizedRule):
			allowed = allowed && !isAllowedResource(uri, false, []string{normalizedRule})
		case isNegated:
			// a negated rule of the other kind leaves the decision unchanged
		case rule == "all":
			allowed = true
		case isRemote && rule == "local":
			// remote resources stay as they are
		case isRemote && rule == "remote":
			allowed = true
		case !isRemote && rule == "remote":
			allowed = false
		case !isRemote && rule == "local":
			allowed = true
		case rule == match:
			allowed = true
		case rule == uri:
			allowed = true
		case isRemote && strings.HasPrefix(absoluteURI, rule):
			allowed = true
		case !isRemote && strings.HasPrefix(absoluteURI, resolvePath(rule)):
			allowed = true
		case isRemote != isRemoteRule(normalizedRule):
			// a rule of the other kind leaves the decision unchanged
		default:
			allowed = false
		}
	}

	return allowed
}

func isRemoteRule(rule string) bool {
	return isRemoteResource(rule) || urlHost("http://"+rule) == rule
}

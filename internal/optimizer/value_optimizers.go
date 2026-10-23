package optimizer

import (
	"math"
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/strutil"
)

var valueOptimizersByName = map[string]ValueOptimizer{
	"color":         optimizeColor,
	"degrees":       optimizeDegrees,
	"fraction":      optimizeFraction,
	"precision":     optimizePrecision,
	"textQuotes":    optimizeTextQuotes,
	"time":          optimizeTime,
	"unit":          optimizeUnit,
	"urlPrefix":     optimizeURLPrefix,
	"urlQuotes":     optimizeURLQuotes,
	"urlWhiteSpace": optimizeURLWhitespace,
	"whiteSpace":    optimizeWhitespace,
	"zero":          optimizeZero,
}

// ValueOptimizerByName returns the value optimizer a variableValueOptimizers
// entry names.
func ValueOptimizerByName(name string) (ValueOptimizer, bool) {
	optimizer, ok := valueOptimizersByName[name]
	return optimizer, ok
}

func noopValueOptimizer(_, value string, _ *options.Options) string {
	return value
}

func levelOne(opts *options.Options) options.Values {
	return opts.Level.Options(options.One)
}

var urlPrefixPattern = regexp.MustCompile(`(?i)^url\(`)

func startsAsURL(value string) bool {
	return urlPrefixPattern.MatchString(value)
}

var zeroDegPattern = regexp.MustCompile(`\(0deg\)`)

func optimizeDegrees(_, value string, opts *options.Options) string {
	if !opts.Compatibility.Bool("properties", "zeroUnits") {
		return value
	}
	if !strings.Contains(value, "0deg") {
		return value
	}
	return zeroDegPattern.ReplaceAllLiteralString(value, "(0)")
}

var (
	expressionPattern          = jsutil.MustCompile(`^expression\(.*\)$`, "")
	anyFunctionPattern         = jsutil.MustCompile(`^(-(?:moz|ms|o|webkit)-[a-z-]+|[a-z-]+)\((.+)\)$`, "")
	tokenSeparatorPattern      = jsutil.MustCompile(`([\s,/])`, "")
	dotZeroPattern             = regexp.MustCompile(`(^|\D)\.0+(\D|$)`)
	fractionPattern            = regexp.MustCompile(`\.([1-9]*)0+(\D|$)`)
	leadingZeroFractionPattern = regexp.MustCompile(`(^|\D)0\.(\d)`)
	minusZeroFractionPattern   = regexp.MustCompile(`([^\w\d-]|^)-0([^.]|$)`)
	zeroPrefixedUnitPattern    = jsutil.MustCompile(`(^|\s)0+([1-9])`, "g")
)

func optimizeFractionsRecursively(value string) string {
	if startsAsURL(value) || expressionPattern.MatchString(value) {
		return value
	}

	functionTokens, ok := jsutil.Exec(anyFunctionPattern, value)
	if !ok {
		return optimizeFractions(value)
	}

	tokens := strutil.SplitKeep(functionTokens.Groups[2], tokenSeparatorPattern)
	for i, t := range tokens {
		tokens[i] = optimizeFractionsRecursively(t)
	}
	return functionTokens.Groups[1] + "(" + strings.Join(tokens, "") + ")"
}

func optimizeFractions(value string) string {
	if !strings.Contains(value, "0") {
		return value
	}

	if strings.Contains(value, "-") {
		value = jsutil.Replace(minusZeroFractionPattern, value, "$10$2", true)
		value = jsutil.Replace(minusZeroFractionPattern, value, "$10$2", true)
	}

	value = jsutil.Replace(zeroPrefixedUnitPattern, value, "$1$2", true)
	value = jsutil.Replace(dotZeroPattern, value, "$10$2", true)
	value = jsutil.ReplaceFunc(fractionPattern, value, true, func(m jsutil.Match) string {
		nonZeroPart, suffix := m.Groups[1], m.Groups[2]
		if nonZeroPart != "" {
			return "." + nonZeroPart + suffix
		}
		return nonZeroPart + suffix
	})
	return jsutil.Replace(leadingZeroFractionPattern, value, "$1.$2", true)
}

func optimizeFraction(_, value string, opts *options.Options) string {
	if !levelOne(opts).Bool("replaceZeroUnits") {
		return value
	}
	return optimizeFractionsRecursively(value)
}

func optimizePrecision(_, value string, opts *options.Options) string {
	precision := opts.Runtime.Precision
	if !precision.Enabled || !strings.Contains(value, ".") {
		return value
	}

	value = jsutil.Replace(precision.DecimalPointPattern, value, "$1$2$3", true)
	return jsutil.ReplaceFunc(precision.ZeroPattern, value, true, func(m jsutil.Match) string {
		integerPart, fractionPart, unit := m.Groups[1], m.Groups[2], m.Groups[3]
		multiplier := precision.Units[unit].Multiplier
		integer := jsutil.ParseInt(integerPart)
		if math.IsNaN(integer) {
			integer = 0
		}
		fraction := jsutil.ParseFloat(fractionPart)
		return jsutil.FormatNumber(jsutil.Round((integer+fraction)*multiplier)/multiplier) + unit
	})
}

var (
	localPrefixPattern         = regexp.MustCompile(`(?i)^local\(`)
	quotedPattern              = jsutil.MustCompile(`^('.*'|".*")$`, "")
	quotedButSafePattern       = regexp.MustCompile(`^['"][a-zA-Z][a-zA-Z\d\-_]+['"]$`)
	genericFontFamilyPattern   = regexp.MustCompile(`^['"](?:cursive|default|emoji|fangsong|fantasy|inherit|initial|math|monospace|revert|revert-layer|sans-serif|serif|system-ui|ui-monospace|ui-rounded|ui-sans-serif|ui-serif|unset)['"]$`)
	timeValuePattern           = regexp.MustCompile(`^(-?[\d.]+)(m?s)$`)
	wholePixelValuePattern     = jsutil.MustCompile(`(?:^|\s|\()(-?\d+)px`, "")
	quotedURLPattern           = jsutil.MustCompile(`^url\(['"].+['"]\)$`, "")
	quotedURLWhitespacePattern = jsutil.MustCompile(`^url\(['"].*[*\s()'"].*['"]\)$`, "")
	quotesPattern              = regexp.MustCompile(`["']`)
	urlDataPattern             = regexp.MustCompile(`^url\(['"]data:[^;]+;charset`)
	urlLineBreakPattern        = regexp.MustCompile(`\\?\n|\\?\r\n`)
	urlWhitespacePrefixPattern = jsutil.MustCompile(`(\()\s+`, "g")
	urlWhitespaceSuffixPattern = jsutil.MustCompile(`\s+(\))`, "g")
)

func optimizeTextQuotes(name, value string, opts *options.Options) string {
	if (name == "font-family" || name == "font") && genericFontFamilyPattern.MatchString(value) {
		return value
	}
	if !levelOne(opts).Bool("removeQuotes") {
		return value
	}
	if !quotedPattern.MatchString(value) && !localPrefixPattern.MatchString(value) {
		return value
	}
	if quotedButSafePattern.MatchString(value) {
		return value[1 : len(value)-1]
	}
	return value
}

func optimizeTime(_, value string, opts *options.Options) string {
	if !levelOne(opts).Bool("replaceTimeUnits") {
		return value
	}
	m, ok := jsutil.Exec(timeValuePattern, value)
	if !ok {
		return value
	}

	val, unit := m.Groups[1], m.Groups[2]
	var newValue string
	if unit == "ms" {
		newValue = jsutil.FormatNumber(jsutil.ParseInt(val)/1000) + "s"
	} else {
		newValue = jsutil.FormatNumber(jsutil.ParseFloat(val)*1000) + "ms"
	}
	if len(newValue) < len(m.Text()) {
		return newValue
	}
	return m.Text()
}

func optimizeUnit(_, value string, opts *options.Options) string {
	if !wholePixelValuePattern.MatchString(value) {
		return value
	}

	shorter := opts.Compatibility.Bool("properties", "shorterLengthUnits")
	return jsutil.ReplaceFunc(wholePixelValuePattern, value, false, func(m jsutil.Match) string {
		match, val := m.Text(), m.Groups[1]
		intVal := jsutil.ParseInt(val)
		if intVal == 0 {
			return match
		}

		newValue := ""
		if shorter && opts.Compatibility.Bool("units", "pt") && math.Mod(intVal*3, 4) == 0 {
			newValue = jsutil.FormatNumber(intVal*3/4) + "pt"
		}
		if shorter && opts.Compatibility.Bool("units", "pc") && math.Mod(intVal, 16) == 0 {
			newValue = jsutil.FormatNumber(intVal/16) + "pc"
		}
		if shorter && opts.Compatibility.Bool("units", "in") && math.Mod(intVal, 96) == 0 {
			newValue = jsutil.FormatNumber(intVal/96) + "in"
		}
		if newValue != "" {
			newValue = match[:strings.Index(match, val)] + newValue
		}
		if newValue != "" && jsutil.Len16(newValue) < jsutil.Len16(match) {
			return newValue
		}
		return match
	})
}

func optimizeURLPrefix(_, value string, opts *options.Options) string {
	if !levelOne(opts).Bool("normalizeUrls") {
		return value
	}
	if !startsAsURL(value) {
		return value
	}
	return urlPrefixPattern.ReplaceAllLiteralString(value, "url(")
}

func optimizeURLQuotes(_, value string, opts *options.Options) string {
	if opts.Compatibility.Bool("properties", "urlQuotes") {
		return value
	}
	if quotedURLPattern.MatchString(value) && !quotedURLWhitespacePattern.MatchString(value) && !urlDataPattern.MatchString(value) {
		return quotesPattern.ReplaceAllLiteralString(value, "")
	}
	return value
}

func optimizeURLWhitespace(_, value string, _ *options.Options) string {
	if !startsAsURL(value) {
		return value
	}
	value = urlLineBreakPattern.ReplaceAllLiteralString(value, "")
	value = jsutil.Replace(urlWhitespacePrefixPattern, value, "$1", true)
	return jsutil.Replace(urlWhitespaceSuffixPattern, value, "$1", true)
}

var (
	calcDivisionWhitespacePattern         = regexp.MustCompile(`\) ?/ ?`)
	lineBreakPattern                      = regexp.MustCompile(`\r?\n`)
	multiWhitespacePattern                = jsutil.MustCompile(`\s+`, "g")
	functionClosingBraceWhitespacePattern = jsutil.MustCompile(`\s+(;?\))`, "g")
	functionOpeningBraceWhitespacePattern = jsutil.MustCompile(`(\(;?)\s+`, "g")
	variableNamePattern                   = jsutil.MustCompile(`^--\S+$`, "")
	variableValuePattern                  = jsutil.MustCompile(`^var\(\s*--\S+\s*\)$`, "")
)

func optimizeWhitespace(name, value string, opts *options.Options) string {
	if !levelOne(opts).Bool("removeWhitespace") {
		return value
	}
	if variableNamePattern.MatchString(name) && !variableValuePattern.MatchString(value) {
		return value
	}
	if !strings.Contains(value, " ") && !strings.Contains(value, "\n") || strings.HasPrefix(value, "expression") {
		return value
	}
	if strings.ContainsAny(value, `'"`) {
		return value
	}

	value = lineBreakPattern.ReplaceAllLiteralString(value, "")
	value = multiWhitespacePattern.ReplaceAllLiteralString(value, " ")

	if strings.Contains(value, "calc") {
		value = calcDivisionWhitespacePattern.ReplaceAllLiteralString(value, ")/ ")
	}

	value = jsutil.Replace(functionOpeningBraceWhitespacePattern, value, "$1", true)
	value = jsutil.Replace(functionClosingBraceWhitespacePattern, value, "$1", true)
	return strings.ReplaceAll(value, ", ", ",")
}

var skipFunctionPattern = regexp.MustCompile(`^(?:-moz-calc|-webkit-calc|calc|rgb|hsl|rgba|hsla|min|max|clamp|expression)\(`)

func removeZerosRecursively(value string, opts *options.Options) string {
	if skipFunctionPattern.MatchString(value) {
		return value
	}

	functionTokens, ok := jsutil.Exec(anyFunctionPattern, value)
	if !ok {
		value = jsutil.Replace(opts.Runtime.UnitsPattern, value, "$10$2", true)
		return jsutil.Replace(opts.Runtime.UnitsPattern, value, "$10$2", true)
	}

	tokens := strutil.SplitKeep(functionTokens.Groups[2], tokenSeparatorPattern)
	for i, t := range tokens {
		tokens[i] = removeZerosRecursively(t, opts)
	}
	return functionTokens.Groups[1] + "(" + strings.Join(tokens, "") + ")"
}

func optimizeZero(name, value string, opts *options.Options) string {
	if !opts.Compatibility.Bool("properties", "zeroUnits") {
		return value
	}
	if strings.Index(value, "%") > 0 && (name == "height" || name == "max-height" || name == "width" || name == "max-width") {
		return value
	}
	return removeZerosRecursively(value, opts)
}

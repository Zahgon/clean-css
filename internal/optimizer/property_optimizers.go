package optimizer

import (
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
)

func valueText(values []*token.Token, i int) (string, bool) {
	if i < 0 || i >= len(values) {
		return "", false
	}
	return values[i].Value, true
}

func allZeros(values []*token.Token) bool {
	return len(values) == 4 && values[0].Value == "0" && values[1].Value == "0" && values[2].Value == "0" && values[3].Value == "0"
}

func optimizeBackgroundProperty(_ string, property *Property, opts *options.Options) {
	values := property.Value
	if !levelOne(opts).Bool("optimizeBackground") {
		return
	}
	if len(values) == 1 && values[0].Value == "none" {
		values[0].Value = "0 0"
	}
	if len(values) == 1 && values[0].Value == "transparent" {
		values[0].Value = "0 0"
	}
}

func optimizeBorderRadiusProperty(_ string, property *Property, opts *options.Options) {
	values := property.Value
	if !levelOne(opts).Bool("optimizeBorderRadius") {
		return
	}

	text := func(i int) string { return values[i].Value }
	switch {
	case len(values) == 3 && text(1) == "/" && text(0) == text(2):
		property.Value = values[:1]
		property.Dirty = true
	case len(values) == 5 && text(2) == "/" && text(0) == text(3) && text(1) == text(4):
		property.Value = values[:2]
		property.Dirty = true
	case len(values) == 7 && text(3) == "/" && text(0) == text(4) && text(1) == text(5) && text(2) == text(6):
		property.Value = values[:3]
		property.Dirty = true
	case len(values) == 9 && text(4) == "/" && text(0) == text(5) && text(1) == text(6) && text(2) == text(7) && text(3) == text(8):
		property.Value = values[:4]
		property.Dirty = true
	}
}

func optimizeBoxShadowProperty(_ string, property *Property, _ *options.Options) {
	// remove multiple zeros
	if allZeros(property.Value) {
		property.Value = property.Value[:2]
		property.Dirty = true
	}
}

var (
	alphaOrChromaFilterPattern    = regexp.MustCompile(`progid:DXImageTransform\.Microsoft\.(Alpha|Chroma)(\W)`)
	noSpaceAfterCommaPattern      = jsutil.MustCompile(`,(\S)`, "g")
	whitespaceAroundEqualsPattern = regexp.MustCompile(` ?= ?`)
)

func optimizeFilterProperty(_ string, property *Property, opts *options.Options) {
	if !opts.Compatibility.Bool("properties", "ieFilters") {
		return
	}
	if !levelOne(opts).Bool("optimizeFilter") {
		return
	}

	first := property.Value[0]
	if len(property.Value) == 1 {
		first.Value = jsutil.ReplaceFunc(alphaOrChromaFilterPattern, first.Value, false, func(m jsutil.Match) string {
			return strings.ToLower(m.Groups[1]) + m.Groups[2]
		})
	}

	first.Value = jsutil.Replace(noSpaceAfterCommaPattern, first.Value, ", $1", true)
	first.Value = whitespaceAroundEqualsPattern.ReplaceAllLiteralString(first.Value, "=")
}

func optimizeFontWeightProperty(_ string, property *Property, opts *options.Options) {
	value := property.Value[0].Value
	if !levelOne(opts).Bool("optimizeFontWeight") {
		return
	}
	switch value {
	case "normal":
		value = "400"
	case "bold":
		value = "700"
	}
	property.Value[0].Value = value
}

func optimizeMarginProperty(_ string, property *Property, opts *options.Options) {
	if !levelOne(opts).Bool("replaceMultipleZeros") {
		return
	}
	// remove multiple zeros
	if allZeros(property.Value) {
		property.Value = property.Value[:1]
		property.Dirty = true
	}
}

func optimizeOutlineProperty(_ string, property *Property, opts *options.Options) {
	values := property.Value
	if !levelOne(opts).Bool("optimizeOutline") {
		return
	}
	if len(values) == 1 && values[0].Value == "none" {
		values[0].Value = "0"
	}
}

func isNegative(values []*token.Token, i int) bool {
	value, ok := valueText(values, i)
	return ok && strings.HasPrefix(value, "-") && jsutil.ParseFloat(value) < 0
}

func optimizePaddingProperty(_ string, property *Property, opts *options.Options) {
	// remove multiple zeros
	if allZeros(property.Value) {
		property.Value = property.Value[:1]
		property.Dirty = true
	}

	// remove negative paddings
	if levelOne(opts).Bool("removeNegativePaddings") &&
		(isNegative(property.Value, 0) || isNegative(property.Value, 1) || isNegative(property.Value, 2) || isNegative(property.Value, 3)) {
		property.Unused = true
	}
}

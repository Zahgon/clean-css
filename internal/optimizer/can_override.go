package optimizer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
)

var vendorPrefixPattern = regexp.MustCompile(`(?:^|\W)(-\w+-)`)

// UniqueVendorPrefixes lists the vendor prefixes found in value, each match
// with the character that precedes it, without repetitions.
func UniqueVendorPrefixes(value string) []string {
	var prefixes []string
	for _, match := range vendorPrefixPattern.FindAllString(value, -1) {
		if !slices.Contains(prefixes, match) {
			prefixes = append(prefixes, match)
		}
	}
	return prefixes
}

// SameVendorPrefixes reports whether two values use the same vendor prefixes.
func SameVendorPrefixes(value1, value2 string) bool {
	return sortedPrefixes(value1) == sortedPrefixes(value2)
}

func sortedPrefixes(value string) string {
	prefixes := UniqueVendorPrefixes(value)
	jsutil.Sort(prefixes, func(a, b string) float64 { return float64(jsutil.CompareUTF16(a, b)) })
	return strings.Join(prefixes, ",")
}

// understandable reports whether two values can be compared at all: they
// share vendor prefixes and, when paired, are both or neither variables.
func understandable(v *Validator, value1, value2 string, isPaired bool) bool {
	if !SameVendorPrefixes(value1, value2) {
		return false
	}
	if isPaired && v.IsVariable(value1) != v.IsVariable(value2) {
		return false
	}
	return true
}

func overrideAnimationIterationCount(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) &&
		!(v.IsAnimationIterationCountKeyword(value2) || v.IsPositiveNumber(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	return v.IsAnimationIterationCountKeyword(value2) || v.IsPositiveNumber(value2)
}

func overrideAnimationName(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !(v.IsAnimationNameKeyword(value2) || v.IsIdentifier(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	return v.IsAnimationNameKeyword(value2) || v.IsIdentifier(value2)
}

func areSameFunction(v *Validator, value1, value2 string) bool {
	if !v.IsFunction(value1) || !v.IsFunction(value2) {
		return false
	}

	function1Name := value1[:max(strings.Index(value1, "("), 0)]
	function2Name := value2[:max(strings.Index(value2, "("), 0)]
	function1Value := substring(value1, len(function1Name)+1, len(value1)-1)
	function2Value := substring(value2, len(function2Name)+1, len(value2)-1)

	if v.IsFunction(function1Value) || v.IsFunction(function2Value) {
		return function1Name == function2Name && areSameFunction(v, function1Value, function2Value)
	}
	return function1Name == function2Name
}

// substring implements String.prototype.substring on byte offsets.
func substring(s string, start, end int) string {
	start = min(max(start, 0), len(s))
	end = min(max(end, 0), len(s))
	if start > end {
		start, end = end, start
	}
	return s[start:end]
}

func overrideBackgroundPosition(v *Validator, value1, value2 string, position int) bool {
	if !understandable(v, value1, value2, true) && !(v.IsBackgroundPositionKeyword(value2) || v.IsGlobal(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if v.IsBackgroundPositionKeyword(value2) || v.IsGlobal(value2) {
		return true
	}
	return overrideUnit(v, value1, value2, position)
}

func overrideBackgroundSize(v *Validator, value1, value2 string, position int) bool {
	if !understandable(v, value1, value2, true) && !(v.IsBackgroundSizeKeyword(value2) || v.IsGlobal(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if v.IsBackgroundSizeKeyword(value2) || v.IsGlobal(value2) {
		return true
	}
	return overrideUnit(v, value1, value2, position)
}

func overrideColor(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !v.IsColor(value2) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if !v.ColorOpacity && (v.IsRgbColor(value1) || v.IsHslColor(value1)) {
		return false
	}
	if !v.ColorOpacity && (v.IsRgbColor(value2) || v.IsHslColor(value2)) {
		return false
	}
	if !v.ColorHexAlpha && (v.IsHexAlphaColor(value1) || v.IsHexAlphaColor(value2)) {
		return false
	}
	if v.IsColor(value1) && v.IsColor(value2) {
		return true
	}
	return sameFunctionOrValue(v, value1, value2)
}

// overrideComponents checks each component of a shorthand with its own check.
func overrideComponents(checks ...OverrideCheck) OverrideCheck {
	return func(v *Validator, value1, value2 string, position int) bool {
		return checks[position](v, value1, value2, position)
	}
}

func overrideFontFamily(v *Validator, value1, value2 string, _ int) bool {
	return understandable(v, value1, value2, true)
}

func overrideImage(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !v.IsImage(value2) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if v.IsImage(value2) {
		return true
	}
	if v.IsImage(value1) {
		return false
	}
	return sameFunctionOrValue(v, value1, value2)
}

func overrideKeyword(group string) OverrideCheck {
	return func(v *Validator, value1, value2 string, _ int) bool {
		if !understandable(v, value1, value2, true) && !v.IsKeyword(group, value2) {
			return false
		}
		if v.IsVariable(value1) && v.IsVariable(value2) {
			return true
		}
		return v.IsKeyword(group, value2)
	}
}

func overrideKeywordWithGlobal(group string) OverrideCheck {
	return func(v *Validator, value1, value2 string, _ int) bool {
		if !understandable(v, value1, value2, true) && !(v.IsKeyword(group, value2) || v.IsGlobal(value2)) {
			return false
		}
		if v.IsVariable(value1) && v.IsVariable(value2) {
			return true
		}
		return v.IsKeyword(group, value2) || v.IsGlobal(value2)
	}
}

func overridePropertyName(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !v.IsIdentifier(value2) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	return v.IsIdentifier(value2)
}

func sameFunctionOrValue(v *Validator, value1, value2 string) bool {
	if areSameFunction(v, value1, value2) {
		return true
	}
	return value1 == value2
}

func overrideTextShadow(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !(v.IsUnit(value2) || v.IsColor(value2) || v.IsGlobal(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	return v.IsUnit(value2) || v.IsColor(value2) || v.IsGlobal(value2)
}

func overrideTime(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !v.IsTime(value2) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if v.IsTime(value1) && !v.IsTime(value2) {
		return false
	}
	if v.IsTime(value2) {
		return true
	}
	if v.IsTime(value1) {
		return false
	}
	if v.IsFunction(value1) && !v.IsPrefixed(value1) && v.IsFunction(value2) && !v.IsPrefixed(value2) {
		return true
	}
	return sameFunctionOrValue(v, value1, value2)
}

func overrideTimingFunction(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !(v.IsTimingFunction(value2) || v.IsGlobal(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	return v.IsTimingFunction(value2) || v.IsGlobal(value2)
}

func overrideUnit(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !v.IsUnit(value2) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if v.IsUnit(value1) && !v.IsUnit(value2) {
		return false
	}
	if v.IsUnit(value2) {
		return true
	}
	if v.IsUnit(value1) {
		return false
	}
	if v.IsFunction(value1) && !v.IsPrefixed(value1) && v.IsFunction(value2) && !v.IsPrefixed(value2) {
		return true
	}
	return sameFunctionOrValue(v, value1, value2)
}

func overrideUnitOrKeywordWithGlobal(group string) OverrideCheck {
	byKeyword := overrideKeywordWithGlobal(group)
	return func(v *Validator, value1, value2 string, position int) bool {
		return overrideUnit(v, value1, value2, position) || byKeyword(v, value1, value2, position)
	}
}

func overrideUnitOrNumber(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !(v.IsUnit(value2) || v.IsNumber(value2)) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	if (v.IsUnit(value1) || v.IsNumber(value1)) && !(v.IsUnit(value2) || v.IsNumber(value2)) {
		return false
	}
	if v.IsUnit(value2) || v.IsNumber(value2) {
		return true
	}
	if v.IsUnit(value1) || v.IsNumber(value1) {
		return false
	}
	if v.IsFunction(value1) && !v.IsPrefixed(value1) && v.IsFunction(value2) && !v.IsPrefixed(value2) {
		return true
	}
	return sameFunctionOrValue(v, value1, value2)
}

func overrideZIndex(v *Validator, value1, value2 string, _ int) bool {
	if !understandable(v, value1, value2, true) && !v.IsZIndex(value2) {
		return false
	}
	if v.IsVariable(value1) && v.IsVariable(value2) {
		return true
	}
	return v.IsZIndex(value2)
}

func overrideTransform(v *Validator, value1, value2 string, _ int) bool {
	return sameFunctionOrValue(v, value1, value2)
}

// Property-specific override checks.
var (
	overrideAnimationDirection   = overrideKeywordWithGlobal("animation-direction")
	overrideAnimationFillMode    = overrideKeyword("animation-fill-mode")
	overrideAnimationPlayState   = overrideKeywordWithGlobal("animation-play-state")
	overrideBackgroundAttachment = overrideKeyword("background-attachment")
	overrideBackgroundClip       = overrideKeywordWithGlobal("background-clip")
	overrideBackgroundOrigin     = overrideKeyword("background-origin")
	overrideBackgroundRepeat     = overrideKeyword("background-repeat")
	overrideBottom               = overrideUnitOrKeywordWithGlobal("bottom")
	overrideBorderCollapse       = overrideKeyword("border-collapse")
	overrideBorderStyle          = overrideKeywordWithGlobal("*-style")
	overrideClear                = overrideKeywordWithGlobal("clear")
	overrideCursor               = overrideKeywordWithGlobal("cursor")
	overrideDisplay              = overrideKeywordWithGlobal("display")
	overrideFloat                = overrideKeywordWithGlobal("float")
	overrideLeft                 = overrideUnitOrKeywordWithGlobal("left")
	overrideFontStretch          = overrideKeywordWithGlobal("font-stretch")
	overrideFontStyle            = overrideKeywordWithGlobal("font-style")
	overrideFontVariant          = overrideKeywordWithGlobal("font-variant")
	overrideFontWeight           = overrideKeywordWithGlobal("font-weight")
	overrideListStyleType        = overrideKeywordWithGlobal("list-style-type")
	overrideListStylePosition    = overrideKeywordWithGlobal("list-style-position")
	overrideOutlineStyle         = overrideKeywordWithGlobal("*-style")
	overrideOverflow             = overrideKeywordWithGlobal("overflow")
	overridePosition             = overrideKeywordWithGlobal("position")
	overrideRight                = overrideUnitOrKeywordWithGlobal("right")
	overrideTextAlign            = overrideKeywordWithGlobal("text-align")
	overrideTextDecoration       = overrideKeywordWithGlobal("text-decoration")
	overrideTextOverflow         = overrideKeywordWithGlobal("text-overflow")
	overrideTop                  = overrideUnitOrKeywordWithGlobal("top")
	overrideVerticalAlign        = overrideUnitOrKeywordWithGlobal("vertical-align")
	overrideVisibility           = overrideKeywordWithGlobal("visibility")
	overrideWhiteSpace           = overrideKeywordWithGlobal("white-space")
)

// overrideUndefined stands in for the check the original configures for
// transition-property inside the transition shorthand, which does not exist:
// calling it fails like calling undefined does.
func overrideUndefined(_ *Validator, _, _ string, _ int) bool {
	panic("overrideCheckers[position] is not a function")
}

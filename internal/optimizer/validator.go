package optimizer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
)

const (
	functionNoVendorPattern = `[A-Z]+(\-|[A-Z]|[0-9])+\(.*?\)`
	functionVendorPattern   = `\-(\-|[A-Z]|[0-9])+\(.*?\)`
	variablePattern         = `var\(\-\-[^\)]+\)`
	functionAnyPattern      = `(` + variablePattern + `|` + functionNoVendorPattern + `|` + functionVendorPattern + `)`
)

var (
	calcRegex               = jsutil.MustCompile(`^(\-moz\-|\-webkit\-)?calc\([^\)]+\)$`, "i")
	functionAnyRegex        = jsutil.MustCompile(`^`+functionAnyPattern+`$`, "i")
	hexAlphaColorRegex      = jsutil.MustCompile(`^#(?:[0-9a-f]{4}|[0-9a-f]{8})$`, "i")
	hslColorRegex           = jsutil.MustCompile(`^hsl\(\s{0,31}[-.]?\d+\s{0,31},\s{0,31}\d*\.?\d+%\s{0,31},\s{0,31}\d*\.?\d+%\s{0,31}\)|hsla\(\s{0,31}[-.]?\d+\s{0,31},\s{0,31}\d*\.?\d+%\s{0,31},\s{0,31}\d*\.?\d+%\s{0,31},\s{0,31}\.?\d+\s{0,31}\)$`, "")
	hslColorWithSpacesRegex = jsutil.MustCompile(`^hsl\(\s{0,31}[-.]?\d+(deg)?\s{1,31}\d*\.?\d+%\s{1,31}\d*\.?\d+%\s{0,31}\)|hsla\(\s{0,31}[-.]?\d+(deg)?\s{1,31}\d*\.?\d+%\s{1,31}\d*\.?\d+%\s{1,31}\/\s{1,31}\d*\.?\d+%?\s{0,31}\)$`, "")
	identifierRegex         = jsutil.MustCompile(`^(-[a-z0-9_][a-z0-9\-_]*|[a-z_][a-z0-9\-_]*)$`, "i")
	namedEntityRegex        = jsutil.MustCompile(`^[a-z]+$`, "i")
	prefixRegex             = jsutil.MustCompile(`^-([a-z0-9]|-)*$`, "i")
	quotedTextRegex         = jsutil.MustCompile(`^("[^"]*"|'[^']*')$`, "i")
	rgbColorRegex           = jsutil.MustCompile(`^rgb\(\s{0,31}[\d]{1,3}\s{0,31},\s{0,31}[\d]{1,3}\s{0,31},\s{0,31}[\d]{1,3}\s{0,31}\)|rgba\(\s{0,31}[\d]{1,3}\s{0,31},\s{0,31}[\d]{1,3}\s{0,31},\s{0,31}[\d]{1,3}\s{0,31},\s{0,31}[.\d]+\s{0,31}\)$`, "i")
	rgbColorWithSpacesRegex = jsutil.MustCompile(`^rgb\(\s{0,31}[\d]{1,3}\s{1,31}[\d]{1,3}\s{1,31}[\d]{1,3}\s{0,31}\)|rgba\(\s{0,31}[\d]{1,3}\s{1,31}[\d]{1,3}\s{1,31}[\d]{1,3}\s{1,31}\/\s{1,31}[\d]*\.?[.\d]+%?\s{0,31}\)$`, "i")
	timeUnitPattern         = jsutil.MustCompile(`\d+(s|ms)`, "")
	timingFunctionRegex     = jsutil.MustCompile(`^(cubic-bezier|steps)\([^)]+\)$`, "")
	urlRegex                = regexp.MustCompile(`(?is)^url\(.+\)$`)
	variableRegex           = jsutil.MustCompile(`^`+variablePattern+`$`, "i")
	eightValueColorRegex    = jsutil.MustCompile(`^#[0-9a-f]{8}$`, "i")
	fourValueColorRegex     = jsutil.MustCompile(`^#[0-9a-f]{4}$`, "i")
	sixValueColorRegex      = jsutil.MustCompile(`^#[0-9a-f]{6}$`, "i")
	threeValueColorRegex    = jsutil.MustCompile(`^#[0-9a-f]{3}$`, "i")
)

var validTimeUnits = []string{"ms", "s"}

var keywords = map[string][]string{
	"^":                         {"inherit", "initial", "unset"},
	"*-style":                   {"auto", "dashed", "dotted", "double", "groove", "hidden", "inset", "none", "outset", "ridge", "solid"},
	"*-timing-function":         {"ease", "ease-in", "ease-in-out", "ease-out", "linear", "step-end", "step-start"},
	"animation-direction":       {"alternate", "alternate-reverse", "normal", "reverse"},
	"animation-fill-mode":       {"backwards", "both", "forwards", "none"},
	"animation-iteration-count": {"infinite"},
	"animation-name":            {"none"},
	"animation-play-state":      {"paused", "running"},
	"background-attachment":     {"fixed", "inherit", "local", "scroll"},
	"background-clip":           {"border-box", "content-box", "inherit", "padding-box", "text"},
	"background-origin":         {"border-box", "content-box", "inherit", "padding-box"},
	"background-position":       {"bottom", "center", "left", "right", "top"},
	"background-repeat":         {"no-repeat", "inherit", "repeat", "repeat-x", "repeat-y", "round", "space"},
	"background-size":           {"auto", "cover", "contain"},
	"border-collapse":           {"collapse", "inherit", "separate"},
	"bottom":                    {"auto"},
	"clear":                     {"both", "left", "none", "right"},
	"color":                     {"transparent"},
	"cursor": {
		"all-scroll", "auto", "col-resize", "crosshair", "default", "e-resize", "help", "move", "n-resize",
		"ne-resize", "no-drop", "not-allowed", "nw-resize", "pointer", "progress", "row-resize", "s-resize",
		"se-resize", "sw-resize", "text", "vertical-text", "w-resize", "wait",
	},
	"display": {
		"block", "inline", "inline-block", "inline-table", "list-item", "none", "table", "table-caption",
		"table-cell", "table-column", "table-column-group", "table-footer-group", "table-header-group",
		"table-row", "table-row-group",
	},
	"float":               {"left", "none", "right"},
	"left":                {"auto"},
	"font":                {"caption", "icon", "menu", "message-box", "small-caption", "status-bar", "unset"},
	"font-size":           {"large", "larger", "medium", "small", "smaller", "x-large", "x-small", "xx-large", "xx-small"},
	"font-stretch":        {"condensed", "expanded", "extra-condensed", "extra-expanded", "normal", "semi-condensed", "semi-expanded", "ultra-condensed", "ultra-expanded"},
	"font-style":          {"italic", "normal", "oblique"},
	"font-variant":        {"normal", "small-caps"},
	"font-weight":         {"100", "200", "300", "400", "500", "600", "700", "800", "900", "bold", "bolder", "lighter", "normal"},
	"line-height":         {"normal"},
	"list-style-position": {"inside", "outside"},
	"list-style-type": {
		"armenian", "circle", "decimal", "decimal-leading-zero", "disc",
		"decimal|disc", // the default value of list-style-type, see the configuration
		"georgian", "lower-alpha", "lower-greek", "lower-latin", "lower-roman", "none", "square",
		"upper-alpha", "upper-latin", "upper-roman",
	},
	"overflow": {"auto", "hidden", "scroll", "visible"},
	"position": {"absolute", "fixed", "relative", "static"},
	"right":    {"auto"},
	"text-align": {
		"center", "justify", "left",
		"left|right", // the default value of text-align, see the configuration
		"right",
	},
	"text-decoration": {"line-through", "none", "overline", "underline"},
	"text-overflow":   {"clip", "ellipsis"},
	"top":             {"auto"},
	"vertical-align":  {"baseline", "bottom", "middle", "sub", "super", "text-bottom", "text-top", "top"},
	"visibility":      {"collapse", "hidden", "visible"},
	"white-space":     {"normal", "nowrap", "pre"},
	"width":           {"inherit", "initial", "medium", "thick", "thin"},
}

var units = []string{"%", "ch", "cm", "em", "ex", "in", "mm", "pc", "pt", "px", "rem", "vh", "vm", "vmax", "vmin", "vw"}

// Validator classifies property values; which units count as valid depends
// on the compatibility options.
type Validator struct {
	ColorOpacity  bool
	ColorHexAlpha bool
	validUnits    []string
}

// NewValidator returns the validator for the given compatibility options.
func NewValidator(compatibility options.Compatibility) *Validator {
	var validUnits []string
	for _, unit := range units {
		if !compatibility.Has("units", unit) || compatibility.Get("units", unit) == true {
			validUnits = append(validUnits, unit)
		}
	}
	if compatibility.Bool("customUnits", "rpx") {
		validUnits = append(validUnits, "rpx")
	}
	return &Validator{
		ColorOpacity:  compatibility.Bool("colors", "opacity"),
		ColorHexAlpha: compatibility.Bool("colors", "hexAlpha"),
		validUnits:    validUnits,
	}
}

// IsKeyword reports whether value is a keyword of the named keyword group.
func (v *Validator) IsKeyword(group, value string) bool {
	return slices.Contains(keywords[group], value)
}

// Keyword group checks.
func (v *Validator) IsAnimationDirectionKeyword(value string) bool {
	return v.IsKeyword("animation-direction", value)
}
func (v *Validator) IsAnimationFillModeKeyword(value string) bool {
	return v.IsKeyword("animation-fill-mode", value)
}
func (v *Validator) IsAnimationIterationCountKeyword(value string) bool {
	return v.IsKeyword("animation-iteration-count", value)
}
func (v *Validator) IsAnimationNameKeyword(value string) bool {
	return v.IsKeyword("animation-name", value)
}
func (v *Validator) IsAnimationPlayStateKeyword(value string) bool {
	return v.IsKeyword("animation-play-state", value)
}
func (v *Validator) IsBackgroundAttachmentKeyword(value string) bool {
	return v.IsKeyword("background-attachment", value)
}
func (v *Validator) IsBackgroundClipKeyword(value string) bool {
	return v.IsKeyword("background-clip", value)
}
func (v *Validator) IsBackgroundOriginKeyword(value string) bool {
	return v.IsKeyword("background-origin", value)
}
func (v *Validator) IsBackgroundPositionKeyword(value string) bool {
	return v.IsKeyword("background-position", value)
}
func (v *Validator) IsBackgroundRepeatKeyword(value string) bool {
	return v.IsKeyword("background-repeat", value)
}
func (v *Validator) IsBackgroundSizeKeyword(value string) bool {
	return v.IsKeyword("background-size", value)
}
func (v *Validator) IsFontKeyword(value string) bool     { return v.IsKeyword("font", value) }
func (v *Validator) IsFontSizeKeyword(value string) bool { return v.IsKeyword("font-size", value) }
func (v *Validator) IsFontStretchKeyword(value string) bool {
	return v.IsKeyword("font-stretch", value)
}
func (v *Validator) IsFontStyleKeyword(value string) bool { return v.IsKeyword("font-style", value) }
func (v *Validator) IsFontVariantKeyword(value string) bool {
	return v.IsKeyword("font-variant", value)
}
func (v *Validator) IsFontWeightKeyword(value string) bool { return v.IsKeyword("font-weight", value) }
func (v *Validator) IsGlobal(value string) bool            { return v.IsKeyword("^", value) }
func (v *Validator) IsLineHeightKeyword(value string) bool { return v.IsKeyword("line-height", value) }
func (v *Validator) IsListStylePositionKeyword(value string) bool {
	return v.IsKeyword("list-style-position", value)
}
func (v *Validator) IsListStyleTypeKeyword(value string) bool {
	return v.IsKeyword("list-style-type", value)
}
func (v *Validator) IsStyleKeyword(value string) bool { return v.IsKeyword("*-style", value) }
func (v *Validator) IsWidth(value string) bool        { return v.IsKeyword("width", value) }

// IsColor reports whether value is a color keyword, hex color, color
// function or a named color.
func (v *Validator) IsColor(value string) bool {
	return value != "auto" &&
		(v.IsKeyword("color", value) || v.IsHexColor(value) || v.IsColorFunction(value) || namedEntityRegex.MatchString(value))
}

// IsColorFunction reports whether value is an rgb(a) or hsl(a) color.
func (v *Validator) IsColorFunction(value string) bool {
	return v.IsRgbColor(value) || v.IsHslColor(value)
}

// IsDynamicUnit reports whether value is a calc() expression.
func (v *Validator) IsDynamicUnit(value string) bool {
	return calcRegex.MatchString(value)
}

// IsFunction reports whether value is a function call.
func (v *Validator) IsFunction(value string) bool {
	return functionAnyRegex.MatchString(value)
}

// IsHexColor reports whether value is a 3, 4, 6 or 8 digit hex color.
func (v *Validator) IsHexColor(value string) bool {
	return threeValueColorRegex.MatchString(value) || fourValueColorRegex.MatchString(value) ||
		sixValueColorRegex.MatchString(value) || eightValueColorRegex.MatchString(value)
}

// IsHslColor reports whether value is an hsl(a) color.
func (v *Validator) IsHslColor(value string) bool {
	return hslColorRegex.MatchString(value) || hslColorWithSpacesRegex.MatchString(value)
}

// IsHexAlphaColor reports whether value is a 4 or 8 digit hex color.
func (v *Validator) IsHexAlphaColor(value string) bool {
	return hexAlphaColorRegex.MatchString(value)
}

// IsIdentifier reports whether value is a CSS identifier.
func (v *Validator) IsIdentifier(value string) bool {
	return identifierRegex.MatchString(value)
}

// IsQuotedText reports whether value is a quoted string.
func (v *Validator) IsQuotedText(value string) bool {
	return quotedTextRegex.MatchString(value)
}

// IsImage reports whether value is none, inherit or a url().
func (v *Validator) IsImage(value string) bool {
	return value == "none" || value == "inherit" || v.IsUrl(value)
}

// IsNamedEntity reports whether value is a word of letters.
func (v *Validator) IsNamedEntity(value string) bool {
	return namedEntityRegex.MatchString(value)
}

// IsNumber reports whether value is a plain number.
func (v *Validator) IsNumber(value string) bool {
	return scanForNumber(value) == len(value)
}

// IsRgbColor reports whether value is an rgb(a) color.
func (v *Validator) IsRgbColor(value string) bool {
	return rgbColorRegex.MatchString(value) || rgbColorWithSpacesRegex.MatchString(value)
}

// IsPrefixed reports whether value is a vendor-prefixed keyword.
func (v *Validator) IsPrefixed(value string) bool {
	return prefixRegex.MatchString(value)
}

// IsPositiveNumber reports whether value is a number that is not negative.
func (v *Validator) IsPositiveNumber(value string) bool {
	return v.IsNumber(value) && jsutil.ParseFloat(value) >= 0
}

// IsVariable reports whether value is a var(--name) reference.
func (v *Validator) IsVariable(value string) bool {
	return variableRegex.MatchString(value)
}

// IsTime reports whether value is a time (or zero, or a function with a time).
func (v *Validator) IsTime(value string) bool {
	numberUpTo := scanForNumber(value)
	return numberUpTo == len(value) && jsutil.ParseInt(value) == 0 ||
		numberUpTo > -1 && slices.Contains(validTimeUnits, sliceFrom(value, numberUpTo+1)) ||
		v.IsFunction(value) && timeUnitPattern.MatchString(value)
}

// IsTimingFunction reports whether value is a timing function.
func (v *Validator) IsTimingFunction(value string) bool {
	return v.IsKeyword("*-timing-function", value) || timingFunctionRegex.MatchString(value)
}

// IsUnit reports whether value is a length in a valid unit (or zero, auto or inherit).
func (v *Validator) IsUnit(value string) bool {
	numberUpTo := scanForNumber(value)
	return numberUpTo == len(value) && jsutil.ParseInt(value) == 0 ||
		numberUpTo > -1 && slices.Contains(v.validUnits, strings.ToLower(sliceFrom(value, numberUpTo+1))) ||
		value == "auto" ||
		value == "inherit"
}

// IsUrl reports whether value is a url().
func (v *Validator) IsUrl(value string) bool {
	return urlRegex.MatchString(value)
}

// IsZIndex reports whether value is a valid z-index.
func (v *Validator) IsZIndex(value string) bool {
	return value == "auto" || v.IsNumber(value) || v.IsGlobal(value)
}

// sliceFrom implements String.prototype.slice with one (possibly negative) argument.
func sliceFrom(value string, start int) string {
	if start < 0 {
		start += len(value)
		if start < 0 {
			start = 0
		}
	}
	if start > len(value) {
		return ""
	}
	return value[start:]
}

// scanForNumber returns how many leading characters of value form a number,
// minus one when the scan stops early, like the original.
func scanForNumber(value string) int {
	hasDot := false
	hasSign := false
	i := 0
	for ; i < len(value); i++ {
		character := value[i]
		switch {
		case i == 0 && (character == '+' || character == '-'):
			hasSign = true
		case i > 0 && hasSign && (character == '+' || character == '-'):
			return i - 1
		case character == '.' && !hasDot:
			hasDot = true
		case character == '.' && hasDot:
			return i - 1
		case character >= '0' && character <= '9':
			continue
		default:
			return i - 1
		}
	}
	return i
}

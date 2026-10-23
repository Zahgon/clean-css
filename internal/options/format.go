package options

import (
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
)

// Places where the format option can add line breaks.
const (
	AfterAtRule      = "afterAtRule"
	AfterBlockBegins = "afterBlockBegins"
	AfterBlockEnds   = "afterBlockEnds"
	AfterComment     = "afterComment"
	AfterProperty    = "afterProperty"
	AfterRuleBegins  = "afterRuleBegins"
	AfterRuleEnds    = "afterRuleEnds"
	BeforeBlockEnds  = "beforeBlockEnds"
	BetweenSelectors = "betweenSelectors"
)

var breakNames = []string{
	AfterAtRule, AfterBlockBegins, AfterBlockEnds, AfterComment, AfterProperty,
	AfterRuleBegins, AfterRuleEnds, BeforeBlockEnds, BetweenSelectors,
}

// Places where the format option can add spaces.
const (
	AroundSelectorRelation = "aroundSelectorRelation"
	BeforeBlockBegins      = "beforeBlockBegins"
	BeforeValue            = "beforeValue"
)

var spaceNames = []string{AroundSelectorRelation, BeforeBlockBegins, BeforeValue}

// systemLineBreak is the line break of the platform the original runs on.
const systemLineBreak = "\n"

// Format is the normalized format option. Breaks holds the text written at
// each break point; Spaces holds the raw space switches.
type Format struct {
	Breaks                     map[string]string
	BreakWith                  string
	IndentBy                   float64
	IndentWith                 string
	Spaces                     any
	WrapAt                     any
	SemicolonAfterLastProperty any
}

// AllowsSpace reports whether a space is written at the given place.
func (f *Format) AllowsSpace(where string) bool {
	return jsutil.Truthy(lookup(f.Spaces, where))
}

// Wraps reports whether long lines are wrapped.
func (f *Format) Wraps() bool {
	return jsutil.Truthy(f.WrapAt)
}

func switches(value bool) Object {
	o := newObject()
	for _, name := range breakNames {
		o.Set(name, value)
	}
	return o
}

func spaceSwitches(value bool) Object {
	o := newObject()
	for _, name := range spaceNames {
		o.Set(name, value)
	}
	return o
}

func formatDefaults() Object {
	return objectFrom(
		"breaks", switches(false),
		"breakWith", systemLineBreak,
		"indentBy", 0.0,
		"indentWith", " ",
		"spaces", spaceSwitches(false),
		"wrapAt", false,
		"semicolonAfterLastProperty", false,
	)
}

// FormatFrom normalizes the format option; it returns nil when output is not
// formatted at all.
func FormatFrom(source any) *Format {
	if jsutil.IsUndefined(source) || source == false {
		return nil
	}

	if o, ok := jsutil.AsObject(source); ok {
		if o.Has("breakWith") {
			source = override(source, objectFrom("breakWith", mapBreakWith(o.Lookup("breakWith"))))
		}
		if o.Has("indentBy") {
			source = override(source, objectFrom("indentBy", jsutil.ParseInt(jsutil.ToString(o.Lookup("indentBy")))))
		}
		if o.Has("indentWith") {
			source = override(source, objectFrom("indentWith", mapIndentWith(o.Lookup("indentWith"))))
		}
		return remapBreaks(override(formatDefaults(), source))
	}

	if s, ok := source.(string); ok {
		switch s {
		case "beautify":
			return remapBreaks(override(formatDefaults(), objectFrom(
				"breaks", switches(true),
				"indentBy", 2.0,
				"spaces", spaceSwitches(true),
			)))
		case "keep-breaks":
			return remapBreaks(override(formatDefaults(), objectFrom(
				"breaks", objectFrom(
					AfterAtRule, true,
					AfterBlockBegins, true,
					AfterBlockEnds, true,
					AfterComment, true,
					AfterRuleEnds, true,
					BeforeBlockEnds, true,
				),
			)))
		}
		return remapBreaks(override(formatDefaults(), formatHash(s)))
	}

	// Any other value keeps the defaults without turning the break switches
	// into text: string concatenation then spells false.
	return formatOf(formatDefaults(), func(value any) string { return jsutil.ToString(value) })
}

func formatHash(s string) Object {
	hash := newObject()
	for _, directive := range strings.Split(s, ";") {
		parts := strings.Split(directive, ":")
		name := parts[0]
		var value any = jsutil.Undefined
		if len(parts) > 1 {
			value = parts[1]
		}

		switch name {
		case "breaks", "spaces":
			hash.Set(name, hashValues(value))
		case "indentBy", "wrapAt":
			hash.Set(name, jsutil.ParseInt(jsutil.ToString(value)))
		case "indentWith":
			hash.Set(name, mapIndentWith(value))
		case "breakWith":
			hash.Set(name, mapBreakWith(value))
		}
	}
	return hash
}

func hashValues(value any) Object {
	hash := newObject()
	s, ok := value.(string)
	if !ok {
		return hash
	}
	for _, directive := range strings.Split(s, ",") {
		parts := strings.Split(directive, "=")
		var v any = jsutil.Undefined
		if len(parts) > 1 {
			v = normalizeSwitch(parts[1])
		}
		hash.Set(parts[0], v)
	}
	return hash
}

func normalizeSwitch(value any) any {
	switch value {
	case "false", "off":
		return false
	case "true", "on":
		return true
	}
	return value
}

func mapBreakWith(value any) any {
	switch value {
	case "windows", "crlf", "\r\n":
		return "\r\n"
	case "unix", "lf", "\n":
		return "\n"
	}
	return systemLineBreak
}

func mapIndentWith(value any) any {
	switch value {
	case "space":
		return " "
	case "tab":
		return "\t"
	}
	return value
}

func remapBreaks(source Object) *Format {
	breakWith := jsutil.ToString(source.Lookup("breakWith"))
	return formatOf(source, func(value any) string {
		switch value {
		case true:
			return breakWith
		case false:
			return ""
		}
		return jsutil.Repeat(breakWith, jsutil.ParseInt(jsutil.ToString(value)))
	})
}

func formatOf(source Object, breakText func(any) string) *Format {
	f := &Format{
		Breaks:                     make(map[string]string, len(breakNames)),
		BreakWith:                  jsutil.ToString(source.Lookup("breakWith")),
		IndentBy:                   jsutil.ToNumber(source.Lookup("indentBy")),
		IndentWith:                 jsutil.ToString(source.Lookup("indentWith")),
		Spaces:                     source.Lookup("spaces"),
		WrapAt:                     source.Lookup("wrapAt"),
		SemicolonAfterLastProperty: source.Lookup("semicolonAfterLastProperty"),
	}
	breaks := source.Lookup("breaks")
	for _, name := range breakNames {
		f.Breaks[name] = breakText(lookup(breaks, name))
		if isPrimitive(breaks) {
			// Setting the remapped text on a primitive is silently ignored, so
			// every break reads back as undefined and is written as that word.
			f.Breaks[name] = "undefined"
		}
	}
	return f
}

// isPrimitive reports whether an option value is a boolean, number or string.
func isPrimitive(value any) bool {
	switch value.(type) {
	case bool, float64, string:
		return true
	}
	return false
}

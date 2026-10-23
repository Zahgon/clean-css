package options

import (
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
)

// Compatibility is the normalized compatibility option: groups (colors,
// customUnits, properties, selectors, units) of flags and values.
type Compatibility struct {
	Object
}

// Get returns option key of group, Undefined when it is not set.
func (c Compatibility) Get(group, key string) any {
	return lookup(c.Lookup(group), key)
}

// Bool returns the truthiness of option key of group.
func (c Compatibility) Bool(group, key string) bool {
	return jsutil.Truthy(c.Get(group, key))
}

// Has reports whether group defines key.
func (c Compatibility) Has(group, key string) bool {
	return has(c.Lookup(group), key)
}

// Strings returns option key of group as a list of strings; items that are
// not strings can never match a name and are left out.
func (c Compatibility) Strings(group, key string) []string {
	list, _ := c.Get(group, key).([]any)
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func strings1(values ...string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func defaultCompatibility() Object {
	return objectFrom(
		"colors", objectFrom(
			"hexAlpha", false,
			"opacity", true,
		),
		"customUnits", objectFrom("rpx", false),
		"properties", objectFrom(
			"backgroundClipMerging", true,
			"backgroundOriginMerging", true,
			"backgroundSizeMerging", true,
			"colors", true,
			"ieBangHack", false,
			"ieFilters", false,
			"iePrefixHack", false,
			"ieSuffixHack", false,
			"merging", true,
			"shorterLengthUnits", false,
			"spaceAfterClosingBrace", true,
			"urlQuotes", true,
			"zeroUnits", true,
		),
		"selectors", objectFrom(
			"adjacentSpace", false,
			"ie7Hack", false,
			"mergeablePseudoClasses", strings1(
				":active", ":after", ":before", ":empty", ":checked", ":disabled", ":empty", ":enabled",
				":first-child", ":first-letter", ":first-line", ":first-of-type", ":focus", ":hover", ":lang",
				":last-child", ":last-of-type", ":link", ":not", ":nth-child", ":nth-last-child",
				":nth-last-of-type", ":nth-of-type", ":only-child", ":only-of-type", ":root", ":target", ":visited",
			),
			"mergeablePseudoElements", strings1("::after", "::before", "::first-letter", "::first-line"),
			"mergeLimit", 8191.0,
			"multiplePseudoMerging", true,
		),
		"units", objectFrom(
			"ch", true,
			"in", true,
			"pc", true,
			"pt", true,
			"rem", true,
			"vh", true,
			"vm", true,
			"vmax", true,
			"vmin", true,
			"vw", true,
		),
	)
}

// compatibilityPresets returns the named compatibility presets.
func compatibilityPresets() map[string]Object {
	all := defaultCompatibility()
	ie11 := merge(all, objectFrom("properties", objectFrom("ieSuffixHack", true)))
	ie10 := merge(all, objectFrom("properties", objectFrom("ieSuffixHack", true)))
	ie9 := merge(all, objectFrom("properties", objectFrom("ieFilters", true, "ieSuffixHack", true)))
	ie8 := merge(ie9, objectFrom(
		"colors", objectFrom("opacity", false),
		"properties", objectFrom(
			"backgroundClipMerging", false,
			"backgroundOriginMerging", false,
			"backgroundSizeMerging", false,
			"iePrefixHack", true,
			"merging", false,
		),
		"selectors", objectFrom(
			"mergeablePseudoClasses", strings1(":after", ":before", ":first-child", ":first-letter", ":focus", ":hover", ":visited"),
			"mergeablePseudoElements", []any{},
		),
		"units", objectFrom(
			"ch", false,
			"rem", false,
			"vh", false,
			"vm", false,
			"vmax", false,
			"vmin", false,
			"vw", false,
		),
	))
	ie7 := merge(ie8, objectFrom(
		"properties", objectFrom("ieBangHack", true),
		"selectors", objectFrom(
			"ie7Hack", true,
			"mergeablePseudoClasses", strings1(":first-child", ":first-letter", ":hover", ":visited"),
		),
	))
	return map[string]Object{"*": all, "ie11": ie11, "ie10": ie10, "ie9": ie9, "ie8": ie8, "ie7": ie7}
}

// merge fills target with the values of source it does not define yet,
// descending into nested groups, and returns target.
func merge(source, target Object) Object {
	for _, key := range source.Keys() {
		value, _ := source.Get(key)
		nested, isObject := jsutil.AsObject(value)
		if target.Has(key) && isObject {
			current, ok := jsutil.AsObject(target.Lookup(key))
			if !ok {
				current = newObject()
			}
			target.Set(key, merge(nested, current))
		} else if value == nil && target.Has(key) && !jsutil.Truthy(target.Lookup(key)) {
			target.Set(key, newObject())
		} else if !target.Has(key) {
			target.Set(key, copyValue(value))
		}
	}
	return target
}

var compatibilityListPattern = regexp.MustCompile(`[,+-]`)

// CompatibilityFrom normalizes the compatibility option: a preset name, a
// preset followed by +group.option / -group.option switches, or an object.
func CompatibilityFrom(source any) Compatibility {
	presets := compatibilityPresets()
	return Compatibility{merge(presets["*"], calculateSource(source, presets))}
}

func calculateSource(source any, presets map[string]Object) Object {
	if o, ok := jsutil.AsObject(source); ok {
		return o.Copy()
	}

	asString := jsutil.ToString(source)
	if !compatibilityListPattern.MatchString(asString) {
		if preset, ok := presets[asString]; ok {
			return preset
		}
		return presets["*"]
	}

	parts := strings.Split(asString, ",")
	template := presets["*"]
	if preset, ok := presets[parts[0]]; ok {
		template = preset
		parts = parts[1:]
	}

	switches := newObject()
	for _, part := range parts {
		isAdd := strings.HasPrefix(part, "+")
		key := strings.Split(part[min(1, len(part)):], ".")
		group := key[0]
		option := "undefined"
		if len(key) > 1 {
			option = key[1]
		}

		groupOptions, ok := jsutil.AsObject(switches.Lookup(group))
		if !ok {
			groupOptions = newObject()
			switches.Set(group, groupOptions)
		}
		groupOptions.Set(option, isAdd)
	}

	return merge(template, switches)
}

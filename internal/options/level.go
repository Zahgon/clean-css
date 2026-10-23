package options

import (
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
)

// Optimization level keys.
const (
	Zero = "0"
	One  = "1"
	Two  = "2"
)

// PrecisionOff is the rounding precision that leaves a unit alone.
const PrecisionOff = "off"

// Level is the normalized level option: the option groups of each enabled
// optimization level.
type Level struct {
	Object
}

// Enabled reports whether an optimization level is enabled.
func (l Level) Enabled(level string) bool {
	return l.Has(level)
}

// Options returns the options of an optimization level.
func (l Level) Options(level string) Values {
	o, ok := jsutil.AsObject(l.Lookup(level))
	if !ok {
		o = newObject()
	}
	return Values{o}
}

var precisionUnits = []string{"ch", "cm", "em", "ex", "in", "mm", "pc", "pt", "px", "q", "rem", "vh", "vmax", "vmin", "vw", "%"}

func precisionDefaults(value any) Object {
	o := newObject()
	for _, unit := range precisionUnits {
		o.Set(unit, value)
	}
	return o
}

func levelDefaults() Object {
	return objectFrom(
		Zero, newObject(),
		One, objectFrom(
			"cleanupCharsets", true,
			"normalizeUrls", true,
			"optimizeBackground", true,
			"optimizeBorderRadius", true,
			"optimizeFilter", true,
			"optimizeFontWeight", true,
			"optimizeOutline", true,
			"removeEmpty", true,
			"removeNegativePaddings", true,
			"removeQuotes", true,
			"removeWhitespace", true,
			"replaceMultipleZeros", true,
			"replaceTimeUnits", true,
			"replaceZeroUnits", true,
			"roundingPrecision", RoundingPrecisionFrom(jsutil.Undefined),
			"selectorsSortingMethod", "standard",
			"specialComments", "all",
			"tidyAtRules", true,
			"tidyBlockScopes", true,
			"tidySelectors", true,
			"variableValueOptimizers", []any{},
		),
		Two, objectFrom(
			"mergeAdjacentRules", true,
			"mergeIntoShorthands", true,
			"mergeMedia", true,
			"mergeNonAdjacentRules", true,
			"mergeSemantically", false,
			"overrideProperties", true,
			"removeEmpty", true,
			"reduceNonAdjacentRules", true,
			"removeDuplicateFontRules", true,
			"removeDuplicateMediaBlocks", true,
			"removeDuplicateRules", true,
			"removeUnusedAtRules", false,
			"restructureRules", false,
			"skipProperties", []any{},
		),
	)
}

// LevelFrom normalizes the level option: a level number, or an object of
// per-level options given as objects, booleans or "name:value;..." strings.
func LevelFrom(source any) Level {
	level := override(levelDefaults(), newObject())

	if jsutil.IsUndefined(source) {
		level.Delete(Two)
		return Level{level}
	}

	if s, ok := source.(string); ok {
		source = jsutil.ParseInt(s)
	}

	if n, ok := source.(float64); ok {
		switch n {
		case 2:
			return Level{level}
		case 1:
			level.Delete(Two)
			return Level{level}
		case 0:
			level.Delete(Two)
			level.Delete(One)
			return Level{level}
		}
	}

	src, ok := jsutil.AsObject(source)
	if !ok {
		src = newObject()
	} else {
		src = convertValuesToHashes(src)
	}

	if one, ok := jsutil.AsObject(src.Lookup(One)); ok && one.Has("roundingPrecision") {
		one.Set("roundingPrecision", RoundingPrecisionFrom(one.Lookup("roundingPrecision")))
	}

	if two, ok := jsutil.AsObject(src.Lookup(Two)); ok && two.Has("skipProperties") {
		if s, ok := two.Lookup("skipProperties").(string); ok {
			two.Set("skipProperties", toList(strings.Split(s, ",")))
		}
	}

	if src.Has(Zero) || src.Has(One) || src.Has(Two) {
		level.Set(Zero, override(level.Lookup(Zero), src.Lookup(Zero)))
	}

	applyAll(level, src, One, "*")
	applyAll(level, src, One, "all")

	if src.Has(One) || src.Has(Two) {
		level.Set(One, override(level.Lookup(One), src.Lookup(One)))
	} else {
		level.Delete(One)
	}

	applyAll(level, src, Two, "*")
	applyAll(level, src, Two, "all")

	if src.Has(Two) {
		level.Set(Two, override(level.Lookup(Two), src.Lookup(Two)))
	} else {
		level.Delete(Two)
	}

	return Level{level}
}

// applyAll applies a "switch every flag" key (* or all) of a level.
func applyAll(level, source Object, name, keyword string) {
	options, ok := jsutil.AsObject(source.Lookup(name))
	if !ok || !options.Has(keyword) {
		return
	}
	level.Set(name, override(level.Lookup(name), levelSwitches(name, normalizeSwitch(options.Lookup(keyword)))))
	options.Delete(keyword)
}

// levelSwitches returns the defaults of a level with every flag set to value.
func levelSwitches(name string, value any) Object {
	options := override(levelDefaults().Lookup(name), newObject())
	for _, key := range options.Keys() {
		if _, isFlag := options.Lookup(key).(bool); isFlag {
			options.Set(key, value)
		}
	}
	return options
}

func convertValuesToHashes(source Object) Object {
	cloned := override(source, newObject())

	for _, name := range []string{Zero, One, Two} {
		if !cloned.Has(name) {
			continue
		}
		value := cloned.Lookup(name)
		if jsutil.IsUndefined(value) || value == false {
			cloned.Delete(name)
		}
		if value == true {
			cloned.Set(name, newObject())
		}
		if s, ok := value.(string); ok && cloned.Has(name) {
			cloned.Set(name, levelHash(s, name))
		}
	}

	return cloned
}

func levelHash(s, name string) Object {
	accumulator := newObject()
	for _, directive := range strings.Split(s, ";") {
		parts := strings.Split(directive, ":")
		var value any = jsutil.Undefined
		if len(parts) > 1 {
			value = normalizeSwitch(parts[1])
		}

		if parts[0] == "*" || parts[0] == "all" {
			accumulator = override(accumulator, levelSwitches(name, value))
		} else {
			accumulator.Set(parts[0], value)
		}
	}
	return accumulator
}

func toList(items []string) []any {
	list := make([]any, len(items))
	for i, item := range items {
		list[i] = item
	}
	return list
}

var integerPattern = regexp.MustCompile(`^\d+$`)

// RoundingPrecisionFrom normalizes the roundingPrecision option into a
// unit-to-precision object; PrecisionOff disables rounding for a unit.
func RoundingPrecisionFrom(source any) Object {
	return override(precisionDefaults(PrecisionOff), buildPrecision(source))
}

func buildPrecision(source any) Object {
	switch v := source.(type) {
	case nil, jsutil.UndefinedValue, bool:
		return newObject()
	case float64:
		if v == -1 {
			return precisionDefaults(PrecisionOff)
		}
		return precisionDefaults(v)
	case string:
		if integerPattern.MatchString(v) {
			return precisionDefaults(jsutil.ParseInt(v))
		}
		if v == PrecisionOff {
			return precisionDefaults(PrecisionOff)
		}
		accumulator := newObject()
		for _, directive := range strings.Split(v, ",") {
			parts := strings.Split(directive, "=")
			var value any = jsutil.ParseInt(lookupPart(parts, 1))
			if f := value.(float64); f != f || f == -1 {
				value = PrecisionOff
			}
			if parts[0] == "*" || parts[0] == "all" {
				accumulator = override(accumulator, precisionDefaults(value))
			} else {
				accumulator.Set(parts[0], value)
			}
		}
		return accumulator
	}
	if o, ok := jsutil.AsObject(source); ok {
		return o
	}
	return newObject()
}

func lookupPart(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return "undefined"
}

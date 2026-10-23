package options

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
)

// Options is the normalized configuration of a minifier instance.
type Options struct {
	Batch            bool
	Compatibility    Compatibility
	ExplicitRebaseTo bool
	Format           *Format
	Inline           []string
	Level            Level
	Rebase           bool
	RebaseTo         string

	// Runtime holds what level 1 derives from the options the first time it
	// runs; it is kept for every later minification with the same options,
	// and so is the count of special comments kept so far.
	Runtime Runtime
}

// Runtime is the state level 1 keeps on the options between minifications.
type Runtime struct {
	Prepared           bool
	UnitsPattern       *regexp.Regexp
	Precision          Precision
	CommentsKept       int
	VariableOptimizers []string
}

// Precision describes the units whose numbers are rounded.
type Precision struct {
	Enabled             bool
	Units               map[string]UnitPrecision
	DecimalPointPattern *regexp.Regexp
	ZeroPattern         *regexp.Regexp
}

// UnitPrecision is the rounding of one unit: numbers are rounded to Value
// decimal places by multiplying with Multiplier.
type UnitPrecision struct {
	Value      any
	Multiplier float64
}

// From normalizes the raw options object (nil means no options).
func From(raw Object) (*Options, error) {
	if raw == nil {
		raw = newObject()
	}
	get := raw.Lookup

	rebaseTo, err := rebaseToFrom(get("rebaseTo"))
	if err != nil {
		return nil, err
	}

	return &Options{
		Batch:            jsutil.Truthy(get("batch")),
		Compatibility:    CompatibilityFrom(get("compatibility")),
		ExplicitRebaseTo: raw.Has("rebaseTo"),
		Format:           FormatFrom(get("format")),
		Inline:           InlineFrom(get("inline")),
		Level:            LevelFrom(get("level")),
		Rebase:           rebaseFrom(get("rebase"), get("rebaseTo")),
		RebaseTo:         rebaseTo,
	}, nil
}

// InlineFrom normalizes the inline option into a list of rules.
func InlineFrom(rules any) []string {
	switch v := rules.(type) {
	case []any:
		list := make([]string, len(v))
		for i, rule := range v {
			list[i] = jsutil.ToString(rule)
		}
		return list
	case bool:
		if !v {
			return []string{"none"}
		}
	case jsutil.UndefinedValue:
		return []string{"local"}
	}
	return strings.Split(jsutil.ToString(rules), ",")
}

func rebaseFrom(rebase, rebaseTo any) bool {
	if !jsutil.IsUndefined(rebaseTo) {
		return true
	}
	if jsutil.IsUndefined(rebase) {
		return false
	}
	return jsutil.Truthy(rebase)
}

func rebaseToFrom(option any) (string, error) {
	if jsutil.Truthy(option) {
		return filepath.Abs(jsutil.ToString(option))
	}
	return os.Getwd()
}

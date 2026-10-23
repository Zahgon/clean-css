// Package optimizer holds the property model shared by the optimizers (the
// wrapped property, the property configuration and the value validator) and
// the level 1 optimizations.
package optimizer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/token"
)

// Context is what the optimizers of one minification share.
type Context struct {
	Options   *options.Options
	Validator *Validator
	Warnings  []string
	// Specificities caches computed selector specificities.
	Specificities map[string][3]int
}

// Warn records a warning.
func (c *Context) Warn(message string) {
	c.Warnings = append(c.Warnings, message)
}

// Hack kinds: the browser hacks a property can carry.
const (
	HackAsterisk   = "asterisk"
	HackBang       = "bang"
	HackBackslash  = "backslash"
	HackUnderscore = "underscore"
)

// Property is a declaration prepared for optimization: its name and values
// with !important and hacks stripped, and, for shorthands, its components.
type Property struct {
	// All is the body the property token lives in and Position its index
	// there; All is nil for properties that are not part of a body.
	All      *[]*token.Token
	Position int

	Block       bool
	Components  []*Property
	Dirty       bool
	Dynamic     bool
	Hack        []string // nil when there is no hack; otherwise its kind and suffix
	Important   bool
	Name        string
	Multiplex   bool
	Optimizable bool
	Shorthand   bool
	Unused      bool
	Value       []*token.Token
}

// HackKind returns the kind of hack, or the empty string.
func (p *Property) HackKind() string {
	if len(p.Hack) == 0 {
		return ""
	}
	return p.Hack[0]
}

// HackSuffix returns the suffix of a backslash hack, or the empty string.
func (p *Property) HackSuffix() string {
	if len(p.Hack) < 2 {
		return ""
	}
	return p.Hack[1]
}

// Token returns the property token this property was wrapped from.
func (p *Property) Token() *token.Token {
	return (*p.All)[p.Position]
}

var (
	bangSuffixPattern        = regexp.MustCompile(`!\w+$`)
	importantTokenPattern    = regexp.MustCompile(`(?i)!important$`)
	importantWordPattern     = regexp.MustCompile(`(?i)important$`)
	suffixBangPattern        = regexp.MustCompile(`!$`)
	variableReferencePattern = jsutil.MustCompile(`var\(--.+\)$`, "")
)

// WrapAll wraps every property token of a body, skipping the names listed in
// skip. The wrapped properties refer back to the body through All.
func WrapAll(properties *[]*token.Token, skip []string) []*Property {
	var wrapped []*Property
	for i := len(*properties) - 1; i >= 0; i-- {
		property := (*properties)[i]
		if property.Kind != token.Property {
			continue
		}
		if skip != nil && slices.Contains(skip, property.Name.Value) {
			continue
		}
		single := WrapSingle(property)
		single.All = properties
		single.Position = i
		wrapped = append([]*Property{single}, wrapped...)
	}
	return wrapped
}

func someVariableReferences(property *token.Token) bool {
	for _, value := range property.Values {
		if value.Kind != token.PropertyValue {
			continue
		}
		if variableReferencePattern.MatchString(value.Value) {
			return true
		}
	}
	return false
}

func isMultiplex(property *token.Token) bool {
	for _, value := range property.Values[1:] {
		if value.Kind == token.PropertyValue && (value.Value == "," || value.Value == "/") {
			return true
		}
	}
	return false
}

// lastPart returns the last part of a property token: its last
// value or, without values, its name.
func lastPart(property *token.Token) *token.Token {
	if len(property.Values) > 0 {
		return property.Values[len(property.Values)-1]
	}
	return property.Name
}

func hackFrom(property *token.Token) []string {
	name := property.Name.Value
	last := lastPart(property).Value

	switch {
	case strings.HasPrefix(name, "_"):
		return []string{HackUnderscore}
	case strings.HasPrefix(name, "*"):
		return []string{HackAsterisk}
	case strings.HasPrefix(last, "!") && !importantWordPattern.MatchString(last):
		return []string{HackBang}
	case strings.Index(last, "!") > 0 && !importantWordPattern.MatchString(last) && bangSuffixPattern.MatchString(last):
		return []string{HackBang}
	case strings.Index(last, `\`) > 0 && jsutil.Len16(last[strings.Index(last, `\`)+1:]) == 1:
		return []string{HackBackslash, last[strings.Index(last, `\`)+1:]}
	case strings.Index(last, `\`) == 0 && jsutil.Len16(last) == 2:
		return []string{HackBackslash, last[1:]}
	}
	return nil
}

func isImportant(property *token.Token) bool {
	if len(property.Values) < 1 {
		return false
	}
	last := property.Values[len(property.Values)-1].Value
	if importantTokenPattern.MatchString(last) {
		return true
	}
	beforeLast := property.Name
	if len(property.Values) > 1 {
		beforeLast = property.Values[len(property.Values)-2]
	}
	return importantWordPattern.MatchString(last) && suffixBangPattern.MatchString(beforeLast.Value)
}

// popPart removes the last part of a property token (its last value, or
// with no values its name, as the original's array pop does).
func popPart(property *token.Token) {
	if len(property.Values) > 0 {
		property.Values = property.Values[:len(property.Values)-1]
	} else {
		property.Name = nil
	}
}

func stripImportant(property *token.Token) {
	last := property.Values[len(property.Values)-1]
	oneButLast := property.Name
	if len(property.Values) > 1 {
		oneButLast = property.Values[len(property.Values)-2]
	}

	if importantTokenPattern.MatchString(last.Value) {
		last.Value = importantTokenPattern.ReplaceAllLiteralString(last.Value, "")
	} else {
		last.Value = jsutil.Replace(importantWordPattern, last.Value, "", false)
		oneButLast.Value = jsutil.Replace(suffixBangPattern, oneButLast.Value, "", false)
	}

	if last.Value == "" {
		popPart(property)
	}
	if oneButLast.Value == "" {
		popPart(property)
	}
}

func stripSuffixHack(property *token.Token, hack []string) {
	last := lastPart(property)
	marker := "!"
	if hack[0] == HackBackslash {
		marker = `\`
	}
	cut := strings.Index(last.Value, marker)
	if cut < 0 {
		cut = 0
	}
	last.Value = jsutil.Trim(last.Value[:cut])
	if last.Value == "" {
		popPart(property)
	}
}

// WrapSingle prepares one property token for optimization, stripping its
// !important flag and hacks from the token.
func WrapSingle(property *token.Token) *Property {
	important := isImportant(property)
	if important {
		stripImportant(property)
	}

	hack := hackFrom(property)
	switch {
	case len(hack) > 0 && (hack[0] == HackAsterisk || hack[0] == HackUnderscore):
		property.Name.Value = property.Name.Value[1:]
	case len(hack) > 0 && (hack[0] == HackBackslash || hack[0] == HackBang):
		stripSuffixHack(property, hack)
	}

	return &Property{
		Block:       property.IsBlockProperty(),
		Dynamic:     someVariableReferences(property),
		Hack:        hack,
		Important:   important,
		Name:        property.Name.Value,
		Multiplex:   len(property.Values) > 1 && isMultiplex(property),
		Optimizable: true,
		Value:       token.Copy(property.Values),
	}
}

// RestoreFromOptimizing writes the optimized properties back into their
// tokens, putting back !important and hacks. When restore is given it
// computes the values of optimizable properties (shorthands from their
// components).
func RestoreFromOptimizing(properties []*Property, restore func(*Property) []*token.Token) {
	for i := len(properties) - 1; i >= 0; i-- {
		property := properties[i]

		if property.Dynamic && property.Important {
			restoreImportant(property)
			continue
		}
		if property.Dynamic || property.Unused {
			continue
		}
		if !property.Dirty && !property.Important && property.Hack == nil {
			continue
		}

		var restored []*token.Token
		if property.Optimizable && restore != nil {
			restored = restore(property)
			property.Value = restored
		} else {
			restored = property.Value
		}

		if property.Important {
			restoreImportant(property)
		}
		if property.Hack != nil {
			restoreHack(property)
		}

		if property.All != nil {
			current := (*property.All)[property.Position]
			current.Name.Value = property.Name
			current.Values = append([]*token.Token{}, restored...)
		}
	}
}

func restoreImportant(property *Property) {
	property.Value[len(property.Value)-1].Value += "!important"
}

func restoreHack(property *Property) {
	switch property.Hack[0] {
	case HackUnderscore:
		property.Name = "_" + property.Name
	case HackAsterisk:
		property.Name = "*" + property.Name
	case HackBackslash:
		property.Value[len(property.Value)-1].Value += `\` + property.Hack[1]
	case HackBang:
		property.Value[len(property.Value)-1].Value += " !ie"
	}
}

// RemoveUnused deletes the tokens of properties marked unused from their bodies.
func RemoveUnused(properties []*Property) {
	for i := len(properties) - 1; i >= 0; i-- {
		property := properties[i]
		if property.Unused {
			all := *property.All
			*property.All = append(all[:property.Position:property.Position], all[property.Position+1:]...)
		}
	}
}

// ShallowClone returns a new property with the same name, importance and
// hack but no values or components.
func ShallowClone(property *Property) *Property {
	cloned := WrapSingle(token.NewProperty(property.Name))
	cloned.Important = property.Important
	cloned.Hack = property.Hack
	cloned.Unused = false
	return cloned
}

// DeepClone returns a copy of the property and its components whose value
// lists can be changed without affecting the original.
func DeepClone(property *Property) *Property {
	cloned := ShallowClone(property)
	for i := len(property.Components) - 1; i >= 0; i-- {
		component := ShallowClone(property.Components[i])
		component.Value = token.Copy(property.Components[i].Value)
		cloned.Components = append([]*Property{component}, cloned.Components...)
	}
	cloned.Dirty = true
	cloned.Value = token.Copy(property.Value)
	return cloned
}

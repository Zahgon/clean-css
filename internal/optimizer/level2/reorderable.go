package level2

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/optimizer"
	"github.com/clean-css/clean-css/internal/token"
	"github.com/clean-css/clean-css/internal/writer"
)

// extractedProperty is a declaration seen from outside its rule: what is
// needed to decide whether it can move past other declarations.
type extractedProperty struct {
	name               string
	value              string
	nameRoot           string
	token              *token.Token
	key                string // name:value
	selector           []*token.Token
	inSpecificSelector bool
}

// clone deep-copies the declaration token and the selector list, like the
// original's array clone.
func (p *extractedProperty) clone() *extractedProperty {
	c := *p
	c.token = p.token.Clone()
	c.selector = token.CloneAll(p.selector)
	return &c
}

var (
	specificSelectorPattern = regexp.MustCompile(`[.+>~]`)
	borderTwoWordsPattern   = regexp.MustCompile(`^border-\w+-\w+$`)
	borderWordPattern       = regexp.MustCompile(`border-\w+`)
	borderOneWordPattern    = regexp.MustCompile(`^border-\w+$`)
	vendorPrefixPattern     = regexp.MustCompile(`^-\w+-`)
	lettersPattern          = regexp.MustCompile(`([a-zA-Z]+)`)
)

// extractProperties lists the declarations of a rule, or of every rule of a
// nested block.
func extractProperties(t *token.Token) []*extractedProperty {
	var properties []*extractedProperty

	switch t.Kind {
	case token.Rule:
		inSpecificSelector := !specificSelectorPattern.MatchString(writer.Rules(t.Scopes))
		for _, property := range t.Body {
			if property.Kind != token.Property {
				continue
			}
			name := property.Name.Value
			if name == "" {
				continue
			}
			value := writer.Value(property)
			properties = append(properties, &extractedProperty{
				name:               name,
				value:              value,
				nameRoot:           findNameRoot(name),
				token:              property,
				key:                name + ":" + value,
				selector:           t.Scopes,
				inSpecificSelector: inSpecificSelector,
			})
		}
	case token.NestedBlock:
		for _, child := range t.Body {
			properties = append(properties, extractProperties(child)...)
		}
	}

	return properties
}

func findNameRoot(name string) string {
	switch {
	case name == "list-style":
		return name
	case strings.Index(name, "-radius") > 0:
		return "border-radius"
	case name == "border-collapse" || name == "border-spacing" || name == "border-image":
		return name
	case strings.HasPrefix(name, "border-") && borderTwoWordsPattern.MatchString(name):
		return borderWordPattern.FindString(name)
	case strings.HasPrefix(name, "border-") && borderOneWordPattern.MatchString(name):
		return "border"
	case strings.HasPrefix(name, "text-"):
		return name
	case name == "-chrome-":
		return name
	}
	return strings.ToLower(lettersPattern.FindString(vendorPrefixPattern.ReplaceAllLiteralString(name, "")))
}

var (
	flexProperties   = regexp.MustCompile(`align-items|box-align|box-pack|flex|justify`)
	borderProperties = regexp.MustCompile(`^border-(top|right|bottom|left|color|style|width|radius)`)
	prefixedName     = regexp.MustCompile(`^-(?:moz|webkit|ms|o)-`)
)

// canReorder reports whether every declaration of left can swap places with
// every declaration of right.
func canReorder(left, right []*extractedProperty, cache map[string][3]int) bool {
	for i := len(right) - 1; i >= 0; i-- {
		for j := len(left) - 1; j >= 0; j-- {
			if !canReorderSingle(left[j], right[i], cache) {
				return false
			}
		}
	}
	return true
}

func canReorderSingle(left, right *extractedProperty, cache map[string][3]int) bool {
	leftName, leftValue, leftNameRoot := left.name, left.value, left.nameRoot
	rightName, rightValue, rightNameRoot := right.name, right.value, right.nameRoot

	if leftName == "font" && rightName == "line-height" || rightName == "font" && leftName == "line-height" {
		return false
	}
	if flexProperties.MatchString(leftName) && flexProperties.MatchString(rightName) {
		return false
	}
	if leftNameRoot == rightNameRoot && unprefixed(leftName) == unprefixed(rightName) &&
		vendorPrefixed(leftName) != vendorPrefixed(rightName) {
		return false
	}
	if leftNameRoot == "border" && borderProperties.MatchString(rightNameRoot) &&
		(leftName == "border" || leftName == rightNameRoot || leftValue != rightValue && sameBorderComponent(leftName, rightName)) {
		return false
	}
	if rightNameRoot == "border" && borderProperties.MatchString(leftNameRoot) &&
		(rightName == "border" || rightName == leftNameRoot || leftValue != rightValue && sameBorderComponent(leftName, rightName)) {
		return false
	}
	if leftNameRoot == "border" && rightNameRoot == "border" && leftName != rightName &&
		(isSideBorder(leftName) && isStyleBorder(rightName) || isStyleBorder(leftName) && isSideBorder(rightName)) {
		return false
	}
	if leftNameRoot != rightNameRoot {
		return true
	}
	if leftName == rightName && (leftValue == rightValue || withDifferentVendorPrefix(leftValue, rightValue)) {
		return true
	}
	if leftName != rightName && leftName != leftNameRoot && rightName != rightNameRoot {
		return true
	}
	if leftName != rightName && leftValue == rightValue {
		return true
	}
	if right.inSpecificSelector && left.inSpecificSelector && !inheritable(leftNameRoot) && !inheritable(rightNameRoot) &&
		!rulesOverlap(right.selector, left.selector, false) {
		return true
	}
	if !specificitiesOverlap(left.selector, right.selector, cache) {
		return true
	}
	return false
}

func vendorPrefixed(name string) bool {
	return prefixedName.MatchString(name)
}

func unprefixed(name string) string {
	return prefixedName.ReplaceAllLiteralString(name, "")
}

func lastDashPart(name string) string {
	parts := strings.Split(name, "-")
	return parts[len(parts)-1]
}

func sameBorderComponent(name1, name2 string) bool {
	return lastDashPart(name1) == lastDashPart(name2)
}

func isSideBorder(name string) bool {
	return name == "border-top" || name == "border-right" || name == "border-bottom" || name == "border-left"
}

func isStyleBorder(name string) bool {
	return name == "border-color" || name == "border-style" || name == "border-width"
}

func withDifferentVendorPrefix(value1, value2 string) bool {
	if !vendorPrefixed(value1) || !vendorPrefixed(value2) {
		return false
	}
	parts1 := strings.Split(value1, "-")
	parts2 := strings.Split(value2, "-")
	if len(parts2) <= 2 {
		return true
	}
	return parts1[1] != parts2[2]
}

func inheritable(name string) bool {
	return name == "font" || name == "line-height" || name == "list-style"
}

var modifierPattern = jsutil.MustCompile(`--.+$`, "")

// rulesOverlap reports whether two selector lists share a selector; in BEM
// mode selectors that differ only in a --modifier suffix overlap too.
func rulesOverlap(rule1, rule2 []*token.Token, bemMode bool) bool {
	for _, scope1 := range rule1 {
		for _, scope2 := range rule2 {
			if scope1.Value == scope2.Value {
				return true
			}
			if bemMode && withoutModifiers(scope1.Value) == withoutModifiers(scope2.Value) {
				return true
			}
		}
	}
	return false
}

func withoutModifiers(scope string) string {
	return jsutil.Replace(modifierPattern, scope, "", false)
}

// specificitiesOverlap reports whether any selector of one list has the same
// specificity as any selector of the other.
func specificitiesOverlap(selector1, selector2 []*token.Token, cache map[string][3]int) bool {
	for _, s1 := range selector1 {
		specificity1 := findSpecificity(s1.Value, cache)
		for _, s2 := range selector2 {
			if specificity1 == findSpecificity(s2.Value, cache) {
				return true
			}
		}
	}
	return false
}

func findSpecificity(selector string, cache map[string][3]int) [3]int {
	if value, ok := cache[selector]; ok {
		return value
	}
	value := specificity(selector)
	cache[selector] = value
	return value
}

var (
	letterPattern    = regexp.MustCompile(`[a-zA-Z]`)
	separatorPattern = jsutil.MustCompile(`[\s,(>~+]`, "")
)

// specificity computes the [ids, classes, types] specificity of a selector
// the way the original approximates it.
func specificity(selector string) [3]int {
	var result [3]int
	var isEscaped, isSingleQuoted, isDoubleQuoted bool
	roundBracketLevel := 0
	couldIntroduceNewTypeSelector := false
	withinNotPseudoClass := false
	wasPseudoClass := false

	for i := 0; i < len(selector); {
		_, size := utf8.DecodeRuneInString(selector[i:])
		character := selector[i : i+size]

		switch {
		case isEscaped:
		case character == "'" && !isDoubleQuoted && !isSingleQuoted:
			isSingleQuoted = true
		case character == "'" && !isDoubleQuoted && isSingleQuoted:
			isSingleQuoted = false
		case character == `"` && !isDoubleQuoted && !isSingleQuoted:
			isDoubleQuoted = true
		case character == `"` && isDoubleQuoted && !isSingleQuoted:
			isDoubleQuoted = false
		case isSingleQuoted || isDoubleQuoted:
			i += size
			continue
		case roundBracketLevel > 0 && !withinNotPseudoClass:
		case character == "(":
			roundBracketLevel++
		case character == ")" && roundBracketLevel == 1:
			roundBracketLevel--
			withinNotPseudoClass = false
		case character == ")":
			roundBracketLevel--
		case character == "#":
			result[0]++
		case character == "." || character == "[":
			result[1]++
		case character == ":" && !wasPseudoClass && !strings.HasPrefix(selector[i:], ":not("):
			result[1]++
			withinNotPseudoClass = false
		case character == ":":
			withinNotPseudoClass = true
		case (i == 0 || couldIntroduceNewTypeSelector) && letterPattern.MatchString(character):
			result[2]++
		}

		isEscaped = character == `\`
		wasPseudoClass = character == ":"
		couldIntroduceNewTypeSelector = !isEscaped && separatorPattern.MatchString(character)
		i += size
	}

	return result
}

// specificities returns the specificity cache of a context.
func specificities(ctx *optimizer.Context) map[string][3]int {
	if ctx.Specificities == nil {
		ctx.Specificities = map[string][3]int{}
	}
	return ctx.Specificities
}

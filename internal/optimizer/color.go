package optimizer

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/options"
	"github.com/clean-css/clean-css/internal/strutil"
)

// namedColors lists the named colors with their hex values, in the order the
// original declares them (the order of the alternations built from it).
var namedColors = [][2]string{
	{"aliceblue", "#f0f8ff"},
	{"antiquewhite", "#faebd7"},
	{"aqua", "#0ff"},
	{"aquamarine", "#7fffd4"},
	{"azure", "#f0ffff"},
	{"beige", "#f5f5dc"},
	{"bisque", "#ffe4c4"},
	{"black", "#000"},
	{"blanchedalmond", "#ffebcd"},
	{"blue", "#00f"},
	{"blueviolet", "#8a2be2"},
	{"brown", "#a52a2a"},
	{"burlywood", "#deb887"},
	{"cadetblue", "#5f9ea0"},
	{"chartreuse", "#7fff00"},
	{"chocolate", "#d2691e"},
	{"coral", "#ff7f50"},
	{"cornflowerblue", "#6495ed"},
	{"cornsilk", "#fff8dc"},
	{"crimson", "#dc143c"},
	{"cyan", "#0ff"},
	{"darkblue", "#00008b"},
	{"darkcyan", "#008b8b"},
	{"darkgoldenrod", "#b8860b"},
	{"darkgray", "#a9a9a9"},
	{"darkgreen", "#006400"},
	{"darkgrey", "#a9a9a9"},
	{"darkkhaki", "#bdb76b"},
	{"darkmagenta", "#8b008b"},
	{"darkolivegreen", "#556b2f"},
	{"darkorange", "#ff8c00"},
	{"darkorchid", "#9932cc"},
	{"darkred", "#8b0000"},
	{"darksalmon", "#e9967a"},
	{"darkseagreen", "#8fbc8f"},
	{"darkslateblue", "#483d8b"},
	{"darkslategray", "#2f4f4f"},
	{"darkslategrey", "#2f4f4f"},
	{"darkturquoise", "#00ced1"},
	{"darkviolet", "#9400d3"},
	{"deeppink", "#ff1493"},
	{"deepskyblue", "#00bfff"},
	{"dimgray", "#696969"},
	{"dimgrey", "#696969"},
	{"dodgerblue", "#1e90ff"},
	{"firebrick", "#b22222"},
	{"floralwhite", "#fffaf0"},
	{"forestgreen", "#228b22"},
	{"fuchsia", "#f0f"},
	{"gainsboro", "#dcdcdc"},
	{"ghostwhite", "#f8f8ff"},
	{"gold", "#ffd700"},
	{"goldenrod", "#daa520"},
	{"gray", "#808080"},
	{"green", "#008000"},
	{"greenyellow", "#adff2f"},
	{"grey", "#808080"},
	{"honeydew", "#f0fff0"},
	{"hotpink", "#ff69b4"},
	{"indianred", "#cd5c5c"},
	{"indigo", "#4b0082"},
	{"ivory", "#fffff0"},
	{"khaki", "#f0e68c"},
	{"lavender", "#e6e6fa"},
	{"lavenderblush", "#fff0f5"},
	{"lawngreen", "#7cfc00"},
	{"lemonchiffon", "#fffacd"},
	{"lightblue", "#add8e6"},
	{"lightcoral", "#f08080"},
	{"lightcyan", "#e0ffff"},
	{"lightgoldenrodyellow", "#fafad2"},
	{"lightgray", "#d3d3d3"},
	{"lightgreen", "#90ee90"},
	{"lightgrey", "#d3d3d3"},
	{"lightpink", "#ffb6c1"},
	{"lightsalmon", "#ffa07a"},
	{"lightseagreen", "#20b2aa"},
	{"lightskyblue", "#87cefa"},
	{"lightslategray", "#778899"},
	{"lightslategrey", "#778899"},
	{"lightsteelblue", "#b0c4de"},
	{"lightyellow", "#ffffe0"},
	{"lime", "#0f0"},
	{"limegreen", "#32cd32"},
	{"linen", "#faf0e6"},
	{"magenta", "#ff00ff"},
	{"maroon", "#800000"},
	{"mediumaquamarine", "#66cdaa"},
	{"mediumblue", "#0000cd"},
	{"mediumorchid", "#ba55d3"},
	{"mediumpurple", "#9370db"},
	{"mediumseagreen", "#3cb371"},
	{"mediumslateblue", "#7b68ee"},
	{"mediumspringgreen", "#00fa9a"},
	{"mediumturquoise", "#48d1cc"},
	{"mediumvioletred", "#c71585"},
	{"midnightblue", "#191970"},
	{"mintcream", "#f5fffa"},
	{"mistyrose", "#ffe4e1"},
	{"moccasin", "#ffe4b5"},
	{"navajowhite", "#ffdead"},
	{"navy", "#000080"},
	{"oldlace", "#fdf5e6"},
	{"olive", "#808000"},
	{"olivedrab", "#6b8e23"},
	{"orange", "#ffa500"},
	{"orangered", "#ff4500"},
	{"orchid", "#da70d6"},
	{"palegoldenrod", "#eee8aa"},
	{"palegreen", "#98fb98"},
	{"paleturquoise", "#afeeee"},
	{"palevioletred", "#db7093"},
	{"papayawhip", "#ffefd5"},
	{"peachpuff", "#ffdab9"},
	{"peru", "#cd853f"},
	{"pink", "#ffc0cb"},
	{"plum", "#dda0dd"},
	{"powderblue", "#b0e0e6"},
	{"purple", "#800080"},
	{"rebeccapurple", "#663399"},
	{"red", "#f00"},
	{"rosybrown", "#bc8f8f"},
	{"royalblue", "#4169e1"},
	{"saddlebrown", "#8b4513"},
	{"salmon", "#fa8072"},
	{"sandybrown", "#f4a460"},
	{"seagreen", "#2e8b57"},
	{"seashell", "#fff5ee"},
	{"sienna", "#a0522d"},
	{"silver", "#c0c0c0"},
	{"skyblue", "#87ceeb"},
	{"slateblue", "#6a5acd"},
	{"slategray", "#708090"},
	{"slategrey", "#708090"},
	{"snow", "#fffafa"},
	{"springgreen", "#00ff7f"},
	{"steelblue", "#4682b4"},
	{"tan", "#d2b48c"},
	{"teal", "#008080"},
	{"thistle", "#d8bfd8"},
	{"tomato", "#ff6347"},
	{"turquoise", "#40e0d0"},
	{"violet", "#ee82ee"},
	{"wheat", "#f5deb3"},
	{"white", "#fff"},
	{"whitesmoke", "#f5f5f5"},
	{"yellow", "#ff0"},
	{"yellowgreen", "#9acd32"},
}

var (
	colorToHex    = map[string]string{}
	colorToName   = map[string]string{}
	toHexPattern  = buildColorPatterns()
	toNamePattern = buildNamePattern()
)

func buildColorPatterns() *regexp.Regexp {
	var names []string
	for _, pair := range namedColors {
		name, hex := pair[0], pair[1]
		if len(name) < len(hex) {
			colorToName[hex] = name
		} else {
			colorToHex[name] = hex
			names = append(names, name)
		}
	}
	return jsutil.MustCompile(`(^| |,|\))(`+strings.Join(names, "|")+`)( |,|\)|$)`, "i")
}

func buildNamePattern() *regexp.Regexp {
	var hexes []string
	for _, pair := range namedColors {
		if len(pair[0]) < len(pair[1]) {
			hexes = append(hexes, pair[1])
		}
	}
	return jsutil.MustCompile(`(`+strings.Join(hexes, "|")+`)([^a-f0-9]|$)`, "i")
}

// shortenHex replaces color names by shorter hex values and hex values by
// shorter names.
func shortenHex(value string) string {
	hasHex := strings.Contains(value, "#")
	hexConverter := func(m jsutil.Match) string {
		return m.Groups[1] + colorToHex[strings.ToLower(m.Groups[2])] + m.Groups[3]
	}
	shortened := jsutil.ReplaceFunc(toHexPattern, value, true, hexConverter)
	if shortened != value {
		shortened = jsutil.ReplaceFunc(toHexPattern, shortened, true, hexConverter)
	}
	if !hasHex {
		return shortened
	}
	return jsutil.ReplaceFunc(toNamePattern, shortened, true, func(m jsutil.Match) string {
		return colorToName[strings.ToLower(m.Groups[1])] + m.Groups[2]
	})
}

// hslToRgb converts an hsl() color; the arguments are the matched texts,
// converted the way JavaScript arithmetic converts strings.
func hslToRgb(hue, saturation, lightness string) [3]int32 {
	h := math.Mod(jsutil.StringToNumber(hue), 360)
	if h < 0 {
		h += 360
	}
	h = float64(jsutil.ToInt32(h)) / 360

	s := jsutil.StringToNumber(saturation)
	if s < 0 {
		s = 0
	} else if s > 100 {
		s = 100
	}
	s = float64(jsutil.ToInt32(s)) / 100

	l := jsutil.StringToNumber(lightness)
	if l < 0 {
		l = 0
	} else if l > 100 {
		l = 100
	}
	l = float64(jsutil.ToInt32(l)) / 100

	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l // achromatic
	} else {
		var q float64
		if l < 0.5 {
			q = float64(l * (1 + s))
		} else {
			q = l + s - float64(l*s)
		}
		p := float64(2*l) - q
		r = hueToRgb(p, q, h+1.0/3)
		g = hueToRgb(p, q, h)
		b = hueToRgb(p, q, h-1.0/3)
	}

	return [3]int32{jsutil.ToInt32(r * 255), jsutil.ToInt32(g * 255), jsutil.ToInt32(b * 255)}
}

func hueToRgb(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	if t < 1.0/6 {
		return p + float64((q-p)*6*t)
	}
	if t < 1.0/2 {
		return q
	}
	if t < 2.0/3 {
		return p + float64((q-p)*(2.0/3-t)*6)
	}
	return p
}

func hexByte(n int32) string {
	hex := strconv.FormatInt(int64(n), 16)
	if len(hex) == 1 {
		return "0" + hex
	}
	return hex
}

func shortenHsl(hue, saturation, lightness string) string {
	rgb := hslToRgb(hue, saturation, lightness)
	return "#" + hexByte(rgb[0]) + hexByte(rgb[1]) + hexByte(rgb[2])
}

func clampChannel(value string) int64 {
	return int64(math.Max(0, math.Min(jsutil.ParseInt(value), 255)))
}

func shortenRgb(red, green, blue string) string {
	color := clampChannel(red)<<16 | clampChannel(green)<<8 | clampChannel(blue)
	hex := "00000" + strconv.FormatInt(color, 16)
	return "#" + hex[len(hex)-6:]
}

var (
	anyColorFunctionPattern    = jsutil.MustCompile(`(rgb|rgba|hsl|hsla)\(([^()]+)\)`, "gi")
	colorPrefixPattern         = jsutil.MustCompile(`#|rgb|hsl`, "gi")
	hexLongPattern             = jsutil.MustCompile(`(^|[^='"])#([0-9a-f]{6})`, "gi")
	hexShortPattern            = jsutil.MustCompile(`(^|[^='"])#([0-9a-f]{3})`, "gi")
	hexValuePattern            = jsutil.MustCompile(`[0-9a-f]`, "i")
	hslPattern                 = jsutil.MustCompile(`hsl\((-?\d+),(-?\d+)%?,(-?\d+)%?\)`, "gi")
	rgbaHslaPattern            = jsutil.MustCompile(`(rgb|hsl)a?\((-?\d+),(-?\d+%?),(-?\d+%?),(0*[1-9]+[0-9]*(\.?\d*)?)\)`, "gi")
	rgbPattern                 = jsutil.MustCompile(`rgb\((-?\d+),(-?\d+),(-?\d+)\)`, "gi")
	transparentFunctionPattern = jsutil.MustCompile(`(?:rgba|hsla)\(0,0%?,0%?,0\)`, "g")
)

// optimizeColor shortens colors: rgb/hsl functions into hex, hex into names
// and names into hex, whichever is shorter.
func optimizeColor(name, value string, opts *options.Options) string {
	if !opts.Compatibility.Bool("properties", "colors") {
		return value
	}

	if !colorPrefixPattern.MatchString(value) {
		return shortenHex(value)
	}

	value = jsutil.ReplaceFunc(rgbaHslaPattern, value, true, func(m jsutil.Match) string {
		if jsutil.ParseInt(m.Groups[5]) >= 1 {
			return m.Groups[1] + "(" + m.Groups[2] + "," + m.Groups[3] + "," + m.Groups[4] + ")"
		}
		return m.Text()
	})
	value = jsutil.ReplaceFunc(rgbPattern, value, true, func(m jsutil.Match) string {
		return shortenRgb(m.Groups[1], m.Groups[2], m.Groups[3])
	})
	value = jsutil.ReplaceFunc(hslPattern, value, true, func(m jsutil.Match) string {
		return shortenHsl(m.Groups[1], m.Groups[2], m.Groups[3])
	})
	value = jsutil.ReplaceFunc(hexLongPattern, value, true, func(m jsutil.Match) string {
		prefix, color := m.Groups[1], m.Groups[2]
		if suffix := jsutil.CharAt(m.Input, m.Index+len(m.Text())); suffix != "" && hexValuePattern.MatchString(suffix) {
			return m.Text()
		}
		if color[0] == color[1] && color[2] == color[3] && color[4] == color[5] {
			return strings.ToLower(prefix + "#" + color[0:1] + color[2:3] + color[4:5])
		}
		return strings.ToLower(prefix + "#" + color)
	})
	value = jsutil.ReplaceFunc(hexShortPattern, value, true, func(m jsutil.Match) string {
		return m.Groups[1] + "#" + strings.ToLower(m.Groups[2])
	})
	value = jsutil.ReplaceFunc(anyColorFunctionPattern, value, true, func(m jsutil.Match) string {
		colorFunction, colorDef := m.Groups[1], m.Groups[2]
		tokens := strings.Split(colorDef, ",")
		lower := strings.ToLower(colorFunction)
		applies := lower == "hsl" && len(tokens) == 3 ||
			lower == "hsla" && len(tokens) == 4 ||
			lower == "rgb" && len(tokens) == 3 && strings.Index(colorDef, "%") > 0 ||
			lower == "rgba" && len(tokens) == 4 && strings.Index(tokens[0], "%") > 0
		if !applies {
			return m.Text()
		}
		if !strings.Contains(tokens[1], "%") {
			tokens[1] += "%"
		}
		if !strings.Contains(tokens[2], "%") {
			tokens[2] += "%"
		}
		return colorFunction + "(" + strings.Join(tokens, ",") + ")"
	})

	if opts.Compatibility.Bool("colors", "opacity") && !strings.Contains(name, "background") {
		before := value
		value = jsutil.ReplaceFunc(transparentFunctionPattern, value, true, func(m jsutil.Match) string {
			layers := strutil.Split(before, ",")
			if strings.Contains(layers[len(layers)-1], "gradient(") {
				return m.Text()
			}
			return "transparent"
		})
	}

	return shortenHex(value)
}

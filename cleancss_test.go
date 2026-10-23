package cleancss_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	cleancss "github.com/clean-css/clean-css"
	"github.com/clean-css/clean-css/internal/jsutil"
)

// rootPlaceholder stands for the repository root in the recorded cases.
const rootPlaceholder = "{{ROOT}}"

// recordedOutput is the result the original produced for a case.
type recordedOutput struct {
	Styles             string   `json:"styles"`
	Warnings           []string `json:"warnings"`
	Errors             []string `json:"errors"`
	InlinedStylesheets []string `json:"inlinedStylesheets"`
	OriginalSize       int      `json:"originalSize"`
	MinifiedSize       int      `json:"minifiedSize"`
	Efficiency         string   `json:"efficiency"`
}

// recordedCase is one minification of the upstream test suites: the
// options, the input and the output (or the outputs of a batch).
type recordedCase struct {
	Options json.RawMessage `json:"options"`
	Input   struct {
		Kind  string          `json:"kind"`
		Value json.RawMessage `json:"value"`
	} `json:"input"`
	Batch   bool            `json:"batch"`
	Output  *recordedOutput `json:"output"`
	Outputs []struct {
		Key    string          `json:"key"`
		Output *recordedOutput `json:"output"`
	} `json:"outputs"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	return root
}

func withRoot(data []byte, root string) []byte {
	return bytes.ReplaceAll(data, []byte(rootPlaceholder), []byte(root))
}

func parseObject(t *testing.T, data []byte) *jsutil.Object[any] {
	t.Helper()
	value, err := jsutil.ParseJSON(data)
	if err != nil {
		t.Fatalf("jsutil.ParseJSON(%s): %v", data, err)
	}
	object, ok := jsutil.AsObject(value)
	if !ok {
		t.Fatalf("jsutil.ParseJSON(%s) = %v, want an object", data, value)
	}
	return object
}

func inputOf(t *testing.T, c recordedCase) cleancss.Input {
	t.Helper()
	switch c.Input.Kind {
	case "text":
		var text string
		if err := json.Unmarshal(c.Input.Value, &text); err != nil {
			t.Fatalf("decoding text input: %v", err)
		}
		return cleancss.Text(text)
	case "paths":
		var paths []string
		if err := json.Unmarshal(c.Input.Value, &paths); err != nil {
			t.Fatalf("decoding paths input: %v", err)
		}
		return cleancss.Paths(paths...)
	}
	hash := parseObject(t, c.Input.Value)
	sources := jsutil.NewObject[string]()
	for _, key := range hash.Keys() {
		sources.Set(key, jsutil.ToString(hash.Lookup(key)))
	}
	return cleancss.Sources(sources)
}

func compareOutput(t *testing.T, got *cleancss.Output, want *recordedOutput) {
	t.Helper()
	if got.Styles != want.Styles {
		t.Errorf("Styles = %q, want %q", got.Styles, want.Styles)
	}
	if !slices.Equal(got.Warnings, want.Warnings) {
		t.Errorf("Warnings = %q, want %q", got.Warnings, want.Warnings)
	}
	if !slices.Equal(got.Errors, want.Errors) {
		t.Errorf("Errors = %q, want %q", got.Errors, want.Errors)
	}
	if !slices.Equal(got.InlinedStylesheets, want.InlinedStylesheets) {
		t.Errorf("InlinedStylesheets = %q, want %q", got.InlinedStylesheets, want.InlinedStylesheets)
	}
	if got.Stats.OriginalSize != want.OriginalSize || got.Stats.MinifiedSize != want.MinifiedSize {
		t.Errorf("Stats sizes = %d/%d, want %d/%d", got.Stats.OriginalSize, got.Stats.MinifiedSize, want.OriginalSize, want.MinifiedSize)
	}
	if efficiency := jsutil.FormatNumber(got.Stats.Efficiency); efficiency != want.Efficiency {
		t.Errorf("Stats.Efficiency = %s, want %s", efficiency, want.Efficiency)
	}
}

// TestUpstreamSuites replays the end-to-end cases of the original vows
// suites (integration, module and the optimizer suites) recorded in
// testdata/upstream.
func TestUpstreamSuites(t *testing.T) {
	root := repositoryRoot(t)
	files, err := filepath.Glob(filepath.Join("testdata", "upstream", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no recorded suites found: %v", err)
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("os.ReadFile(%s): %v", file, err)
		}
		var cases []recordedCase
		if err := json.Unmarshal(withRoot(data, root), &cases); err != nil {
			t.Fatalf("decoding %s: %v", file, err)
		}

		suite := strings.TrimSuffix(filepath.Base(file), ".json")
		t.Run(suite, func(t *testing.T) {
			for i, c := range cases {
				t.Run(caseName(i, c), func(t *testing.T) {
					runRecordedCase(t, c)
				})
			}
		})
	}
}

func caseName(i int, c recordedCase) string {
	name := string(c.Input.Value)
	if len(name) > 40 {
		name = name[:40]
	}
	return strconv.Itoa(i) + " " + name
}

func runRecordedCase(t *testing.T, c recordedCase) {
	minifier, err := cleancss.New(parseObject(t, c.Options))
	if err != nil {
		t.Fatalf("cleancss.New(%s): %v", c.Options, err)
	}
	input := inputOf(t, c)

	if !c.Batch {
		compareOutput(t, minifier.Minify(input), c.Output)
		return
	}

	outputs := minifier.MinifyBatch(input)
	keys := outputs.Keys()
	if len(keys) != len(c.Outputs) {
		t.Fatalf("MinifyBatch returned keys %q, want %d results", keys, len(c.Outputs))
	}
	for i, want := range c.Outputs {
		if keys[i] != want.Key {
			t.Fatalf("MinifyBatch key %d = %q, want %q", i, keys[i], want.Key)
		}
		got, _ := outputs.Get(want.Key)
		compareOutput(t, got, want.Output)
	}
}

// fixtureFormat returns the format the batch suite minifies a fixture with.
func fixtureFormat(name string) string {
	if name == "issue-1119" {
		return "beautify"
	}
	return "keep-breaks"
}

// TestFixtures minifies every stylesheet of test/fixtures like the original
// batch suite and compares the result with its -min.css counterpart line by
// line.
func TestFixtures(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("test", "fixtures", "*.css"))
	if err != nil || len(inputs) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}

	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".css")
		if strings.HasSuffix(name, "-min") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			expected, err := os.ReadFile(filepath.Join("test", "fixtures", name+"-min.css"))
			if err != nil {
				t.Fatalf("os.ReadFile: %v", err)
			}

			compatibility := "*"
			if strings.Index(name, "ie7") > 0 {
				compatibility = "ie7"
			}
			opts := jsutil.NewObject[any]()
			opts.Set("compatibility", compatibility)
			opts.Set("format", fixtureFormat(name))
			two := jsutil.NewObject[any]()
			two.Set("restructureRules", true)
			level := jsutil.NewObject[any]()
			level.Set("2", two)
			opts.Set("level", level)
			opts.Set("rebase", true)

			minifier, err := cleancss.New(opts)
			if err != nil {
				t.Fatalf("cleancss.New: %v", err)
			}
			output := minifier.Minify(cleancss.Text(`@import "` + filepath.ToSlash(input) + `";`))

			wantLines := strings.Split(jsutil.Trim(string(expected)), "\n")
			gotLines := strings.Split(output.Styles, "\n")
			if len(gotLines) != len(wantLines) {
				t.Fatalf("Minify(%s) has %d lines, want %d", input, len(gotLines), len(wantLines))
			}
			for i := range wantLines {
				if gotLines[i] != wantLines[i] {
					t.Fatalf("Minify(%s) line %d = %q, want %q", input, i+1, gotLines[i], wantLines[i])
				}
			}
		})
	}
}

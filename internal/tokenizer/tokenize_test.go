package tokenizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/clean-css/clean-css/internal/token"
)

// tokenizerCase is one case of the original tokenizer suite: the source,
// its name, the expected token tree in the original's array form and the
// expected warnings.
type tokenizerCase struct {
	Input    string   `json:"input"`
	Source   string   `json:"source"`
	Tokens   []any    `json:"tokens"`
	Warnings []string `json:"warnings"`
}

// asArrays converts tokens into the nested arrays the original tokenizer
// returns, decoded from JSON so that they compare with the recorded trees.
func asArrays(t *testing.T, tokens []*token.Token) []any {
	t.Helper()
	data, err := json.Marshal(arrays(tokens))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var out []any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return out
}

func arrays(tokens []*token.Token) []any {
	out := []any{}
	for _, t := range tokens {
		out = append(out, array(t))
	}
	return out
}

func positions(t *token.Token) []any {
	out := []any{}
	for _, p := range t.Positions {
		var source any
		if p.Source != "" {
			source = p.Source
		}
		out = append(out, []any{p.Line, p.Column, source})
	}
	return out
}

func array(t *token.Token) any {
	switch t.Kind {
	case token.Rule, token.NestedBlock, token.AtRuleBlock:
		return []any{t.Kind.String(), arrays(t.Scopes), arrays(t.Body)}
	case token.PropertyBlock:
		return []any{t.Kind.String(), arrays(t.Body)}
	case token.Property:
		out := []any{t.Kind.String(), array(t.Name)}
		for _, v := range t.Values {
			out = append(out, array(v))
		}
		return out
	}
	return []any{t.Kind.String(), t.Value, positions(t)}
}

// TestTokenize replays the cases of the original tokenizer suite recorded in
// testdata/tokenize.json.
func TestTokenize(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tokenize.json"))
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	var cases []tokenizerCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decoding testdata: %v", err)
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var warnings []string
			got := asArrays(t, Tokenize(c.Input, Context{Source: c.Source, Warnings: &warnings}))
			if !reflect.DeepEqual(got, c.Tokens) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(c.Tokens)
				t.Errorf("Tokenize(%q) = %s, want %s", c.Input, gotJSON, wantJSON)
			}
			if !slices.Equal(warnings, c.Warnings) && !(len(warnings) == 0 && len(c.Warnings) == 0) {
				t.Errorf("Tokenize(%q) warnings = %q, want %q", c.Input, warnings, c.Warnings)
			}
		})
	}
}

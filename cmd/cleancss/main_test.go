package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCommand(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run(args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestStandardInput(t *testing.T) {
	code, out, errOut := runCommand(t, "a{color:#ff0000}}", "--stats")
	if code != 0 {
		t.Fatalf("exit status = %d, want 0", code)
	}
	if out != "a{color:red}" {
		t.Errorf("stdout = %q, want %q", out, "a{color:red}")
	}
	want := "WARNING: Unexpected '}' at 1:16.\n" +
		"WARNING: Invalid character(s) '}' at 1:16. Ignoring.\n" +
		"originalSize: 17\nminifiedSize: 12\nefficiency: 0.2941176470588235\ninlinedStylesheets: \n"
	if errOut != want {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestMissingImportIsAnError(t *testing.T) {
	code, out, errOut := runCommand(t, `@import "idontexist.css";a{color:blue}`)
	if code != exitErrors {
		t.Fatalf("exit status = %d, want %d", code, exitErrors)
	}
	if out != "a{color:#00f}" {
		t.Errorf("stdout = %q, want %q", out, "a{color:#00f}")
	}
	if want := "ERROR: Ignoring local @import of \"idontexist.css\" as resource is missing.\n"; errOut != want {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestBatch(t *testing.T) {
	dir := t.TempDir()
	for name, css := range map[string]string{"one.css": "a{margin:0px}", "two.css": "b{color:#fff}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(css), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
	}
	code, out, errOut := runCommand(t, "", "-O", `{"batch":true}`, "--", filepath.Join(dir, "two.css"), filepath.Join(dir, "one.css"))
	if code != 0 || errOut != "" {
		t.Fatalf("exit status = %d, stderr = %q, want 0 and no output", code, errOut)
	}
	want := "== " + filepath.Join(dir, "two.css") + "\nb{color:#fff}\n== " + filepath.Join(dir, "one.css") + "\na{margin:0}\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"cleancss: unknown flag --bogus\n":                                {"--bogus"},
		"cleancss: flag -O needs a value\n":                               {"-O"},
		"cleancss: options is not valid JSON\n":                           {"-O", "{bad"},
		"cleancss: options must be a JSON object\n":                       {"-O", "[1]"},
		"cleancss: FILE arguments cannot be combined with --input-hash\n": {"--input-hash", "h.json", "a.css"},
		"cleancss: cannot read /nonexistent/options.json\n":               {"--options-file", "/nonexistent/options.json"},
	}
	for want, args := range cases {
		code, out, errOut := runCommand(t, "", args...)
		if code != exitUsage || out != "" || errOut != want {
			t.Errorf("cleancss %q = %d, %q, %q; want %d, no output, %q", args, code, out, errOut, exitUsage, want)
		}
	}
}

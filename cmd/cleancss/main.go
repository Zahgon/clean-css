// Command cleancss minifies CSS with the clean-css library.
//
//	cleancss [flags] [FILE...]
//	  -O, --options JSON       clean-css options as a JSON object (default {}); repeatable, the last one wins
//	      --options-file FILE  read the options object from a JSON file ("-" reads standard input)
//	      --input-hash FILE    minify a hash input: FILE holds a JSON object mapping each source path to an
//	                           object whose "styles" member is that source's CSS text
//	      --stats              print the statistics after the warnings and errors
//	      --                   ends the flags; every later argument is a FILE
//
// With no FILE and no --input-hash, the CSS read from standard input is
// minified. With FILEs, the files are minified in order as one stylesheet.
//
// The minified CSS is written to standard output unchanged. Standard error
// gets one "WARNING: <text>" line per warning, then one "ERROR: <text>" line
// per error, then with --stats the lines "originalSize: <n>", "minifiedSize:
// <n>", "efficiency: <number>" and "inlinedStylesheets: <comma-separated
// list>". The exit status is 1 when there are errors and 0 otherwise.
//
// In batch mode (the "batch" option with FILEs or --input-hash) every source
// is minified on its own: standard output gets "== <key>", the CSS and a
// newline per source, and each standard error line is prefixed with "<key>: ".
//
// Usage errors print one "cleancss: " line and exit with status 2; an input
// the minifier cannot process prints "cleancss: internal error: <message>"
// and exits with status 70.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	cleancss "github.com/clean-css/clean-css"
	"github.com/clean-css/clean-css/internal/jsutil"
	"github.com/clean-css/clean-css/internal/reader"
)

const (
	exitErrors   = 1
	exitUsage    = 2
	exitInternal = 70
)

// usageError is a problem with the command line or the files it names.
type usageError struct {
	message string
}

func (e *usageError) Error() string {
	return e.message
}

type config struct {
	options   *jsutil.Object[any]
	inputHash string
	hasHash   bool
	stats     bool
	files     []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args, stdin)
	var input cleancss.Input
	if err == nil {
		input, err = readInput(cfg, stdin)
	}
	var usage *usageError
	if errors.As(err, &usage) {
		fmt.Fprintf(stderr, "cleancss: %s\n", usage.message)
		return exitUsage
	}
	if err != nil {
		fmt.Fprintf(stderr, "cleancss: internal error: %s\n", err)
		return exitInternal
	}

	minifier, err := cleancss.New(cfg.options)
	if err != nil {
		fmt.Fprintf(stderr, "cleancss: internal error: %s\n", err)
		return exitInternal
	}

	var out strings.Builder
	var lines []string
	failed := false
	isBatch := minifier.Batch() && (len(cfg.files) > 0 || cfg.hasHash)

	if isBatch {
		results := minifier.MinifyBatch(input)
		for _, key := range results.Keys() {
			result, _ := results.Get(key)
			fmt.Fprintf(&out, "== %s\n%s\n", key, result.Styles)
			lines = report(result, cfg.stats, key+": ", lines)
			failed = failed || len(result.Errors) > 0
		}
	} else {
		result := minifier.Minify(input)
		out.WriteString(result.Styles)
		lines = report(result, cfg.stats, "", lines)
		failed = len(result.Errors) > 0
	}

	if _, err := io.WriteString(stdout, out.String()); err != nil {
		fmt.Fprintf(stderr, "cleancss: %s\n", err)
		return exitErrors
	}
	if len(lines) > 0 {
		if _, err := io.WriteString(stderr, strings.Join(lines, "\n")+"\n"); err != nil {
			return exitErrors
		}
	}
	if failed {
		return exitErrors
	}
	return 0
}

func parseArgs(args []string, stdin io.Reader) (*config, error) {
	cfg := &config{options: jsutil.NewObject[any]()}
	value := func(i int, flag string) (string, error) {
		if i+1 >= len(args) {
			return "", &usageError{"flag " + flag + " needs a value"}
		}
		return args[i+1], nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			cfg.files = append(cfg.files, args[i+1:]...)
			break
		}
		switch arg {
		case "-O", "--options":
			text, err := value(i, arg)
			if err != nil {
				return nil, err
			}
			if cfg.options, err = parseObject(text, "options"); err != nil {
				return nil, err
			}
			i++
		case "--options-file":
			file, err := value(i, arg)
			if err != nil {
				return nil, err
			}
			text, err := readText(file, stdin)
			if err != nil {
				return nil, err
			}
			if cfg.options, err = parseObject(text, "options file"); err != nil {
				return nil, err
			}
			i++
		case "--input-hash":
			file, err := value(i, arg)
			if err != nil {
				return nil, err
			}
			cfg.inputHash, cfg.hasHash = file, true
			i++
		case "--stats":
			cfg.stats = true
		default:
			if len(arg) > 1 && arg[0] == '-' {
				return nil, &usageError{"unknown flag " + arg}
			}
			cfg.files = append(cfg.files, arg)
		}
	}

	if cfg.hasHash && len(cfg.files) > 0 {
		return nil, &usageError{"FILE arguments cannot be combined with --input-hash"}
	}
	return cfg, nil
}

// readText reads a file ("-" for standard input) as UTF-8 text.
func readText(file string, stdin io.Reader) (string, error) {
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return "", &usageError{"cannot read " + file}
	}
	return reader.DecodeUTF8(data), nil
}

func parseObject(text, what string) (*jsutil.Object[any], error) {
	parsed, err := jsutil.ParseJSON([]byte(text))
	if err != nil {
		return nil, &usageError{what + " is not valid JSON"}
	}
	object, ok := jsutil.AsObject(parsed)
	if !ok {
		return nil, &usageError{what + " must be a JSON object"}
	}
	return object, nil
}

func readInput(cfg *config, stdin io.Reader) (cleancss.Input, error) {
	if cfg.hasHash {
		text, err := readText(cfg.inputHash, stdin)
		if err != nil {
			return cleancss.Input{}, err
		}
		hash, err := parseObject(text, "input hash")
		if err != nil {
			return cleancss.Input{}, err
		}
		sources := jsutil.NewObject[string]()
		for _, key := range hash.Keys() {
			entry, _ := jsutil.AsObject(hash.Lookup(key))
			styles, ok := entry.Lookup("styles").(string)
			if !ok {
				return cleancss.Input{}, fmt.Errorf("the styles of %q are not a string", key)
			}
			sources.Set(key, styles)
		}
		return cleancss.Sources(sources), nil
	}
	if len(cfg.files) > 0 {
		return cleancss.Paths(cfg.files...), nil
	}
	text, err := readText("-", stdin)
	if err != nil {
		return cleancss.Input{}, err
	}
	return cleancss.Text(text), nil
}

// report appends the warnings, errors and, when asked, the statistics of a
// result to lines, each line starting with prefix.
func report(result *cleancss.Output, stats bool, prefix string, lines []string) []string {
	for _, warning := range result.Warnings {
		lines = append(lines, prefix+"WARNING: "+warning)
	}
	for _, e := range result.Errors {
		lines = append(lines, prefix+"ERROR: "+e)
	}
	if stats {
		lines = append(lines,
			fmt.Sprintf("%soriginalSize: %d", prefix, result.Stats.OriginalSize),
			fmt.Sprintf("%sminifiedSize: %d", prefix, result.Stats.MinifiedSize),
			prefix+"efficiency: "+jsutil.FormatNumber(result.Stats.Efficiency),
			prefix+"inlinedStylesheets: "+strings.Join(result.InlinedStylesheets, ","),
		)
	}
	return lines
}

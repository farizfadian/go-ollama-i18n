package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const version = "0.1.0"

type options struct {
	dir         string
	source      string
	target      string
	model       string
	host        string
	concurrency int
	timeout     time.Duration
	noCache     bool
	dryRun      bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("ollama-i18n", flag.ContinueOnError)
	var opt options
	var showVersion bool

	fs.StringVar(&opt.dir, "dir", "", "directory containing locale files (required)")
	fs.StringVar(&opt.dir, "d", "", "shorthand for --dir")
	fs.StringVar(&opt.source, "source", "", "source locale name without extension, e.g. en (required)")
	fs.StringVar(&opt.source, "s", "", "shorthand for --source")
	fs.StringVar(&opt.target, "target", "", "target locale name; if omitted, all other locales in --dir are processed")
	fs.StringVar(&opt.target, "t", "", "shorthand for --target")
	fs.StringVar(&opt.model, "model", "translategemma", "Ollama model to use")
	fs.StringVar(&opt.model, "m", "translategemma", "shorthand for --model")
	fs.StringVar(&opt.host, "host", defaultHost(), "Ollama base URL")
	fs.IntVar(&opt.concurrency, "concurrency", 4, "number of concurrent translation requests")
	fs.DurationVar(&opt.timeout, "timeout", 120*time.Second, "per-request timeout")
	fs.BoolVar(&opt.noCache, "no-cache", false, "retranslate every key, ignoring existing translations")
	fs.BoolVar(&opt.dryRun, "dry-run", false, "report what would change without calling Ollama or writing files")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "shorthand for --version")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if showVersion {
		fmt.Println("go-ollama-i18n", version)
		return nil
	}
	if opt.dir == "" || opt.source == "" {
		fs.Usage()
		return errors.New("--dir and --source are required")
	}

	srcPath := filepath.Join(opt.dir, opt.source+".json")
	src, err := loadLocale(srcPath)
	if err != nil {
		return fmt.Errorf("reading source %s: %w", srcPath, err)
	}

	targets, err := resolveTargets(opt)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Println("no target locales found.")
		return nil
	}

	var provider Provider = NewOllamaProvider(opt.host, opt.model, opt.timeout)
	if opt.dryRun {
		provider = dryRunProvider{}
	}
	ctx := context.Background()
	srcLang := languageName(opt.source)

	for _, name := range targets {
		path := filepath.Join(opt.dir, name+".json")
		existing, err := loadLocaleOrEmpty(path)
		if err != nil {
			return fmt.Errorf("reading target %s: %w", path, err)
		}

		lang := languageName(name)
		out, stats, err := Translate(ctx, provider, src, existing, srcLang, lang, opt.noCache, opt.concurrency)
		if err != nil {
			return fmt.Errorf("translating %s: %w", name, err)
		}

		action := "wrote"
		if opt.dryRun {
			action = "[dry-run] would translate"
		} else if err := writeLocale(path, out); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Printf("%-7s %-22s %s  (translated %d, kept %d, copied %d)\n",
			name, "→ "+lang, action, stats.Translated, stats.Kept, stats.Copied)
	}
	return nil
}

func defaultHost() string {
	if h := os.Getenv("OLLAMA_HOST"); h != "" {
		if !strings.HasPrefix(h, "http") {
			return "http://" + h
		}
		return h
	}
	return "http://localhost:11434"
}

// resolveTargets returns the list of locale names to process.
func resolveTargets(opt options) ([]string, error) {
	if opt.target != "" {
		return []string{opt.target}, nil
	}
	entries, err := os.ReadDir(opt.dir)
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", opt.dir, err)
	}
	var targets []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if name == opt.source {
			continue
		}
		targets = append(targets, name)
	}
	return targets, nil
}

func loadLocale(path string) (*OrderedMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := NewOrderedMap()
	if len(strings.TrimSpace(string(data))) == 0 {
		return m, nil // empty file is a valid empty locale
	}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, err
	}
	return m, nil
}

// loadLocaleOrEmpty behaves like loadLocale but treats a missing file as empty,
// so --target can create a brand-new locale.
func loadLocaleOrEmpty(path string) (*OrderedMap, error) {
	m, err := loadLocale(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewOrderedMap(), nil
	}
	return m, err
}

func writeLocale(path string, m *OrderedMap) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// dryRunProvider echoes a marker instead of calling Ollama, so users can verify
// which keys are missing before committing to a real run.
type dryRunProvider struct{}

func (dryRunProvider) Name() string { return "dry-run" }
func (dryRunProvider) Translate(_ context.Context, text, sourceLang, targetLang string) (string, error) {
	return fmt.Sprintf("[%s] %s", targetLang, text), nil
}

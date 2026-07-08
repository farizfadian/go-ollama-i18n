# go-ollama-i18n

A tiny, zero-dependency CLI for translating i18n locale JSON files with a local
[Ollama](https://ollama.com) model — a Go port of
[`fkapsahili/ollama-i18n`](https://github.com/fkapsahili/ollama-i18n).

Same idea: use `en.json` as the source of truth, fill in any missing keys in
the other locale files using a small local LLM, and keep what's already
translated. No API keys, no per-token cost, runs offline.

## Why a Go version

- **Single static binary** — `go build`, copy the file, done. No Node/npm runtime.
- **Stdlib only** — nothing in `go.mod` except the standard library.
- **Order-preserving output** — keys keep their source order, so locale diffs
  stay clean (Go's default JSON sorts map keys; this tool doesn't).
- **Concurrent** — translates multiple keys at once (`--concurrency`).
- **Pluggable provider** — Ollama is one implementation of a `Provider`
  interface; a Claude/OpenAI backend can drop in without touching merge logic.
- **`--dry-run`** — see exactly which keys are missing before any model call.

## Prerequisites

- Go 1.22+ (to build)
- [Ollama](https://ollama.com) running locally with a model pulled. The default
  is [TranslateGemma](https://ollama.com/library/translategemma), a Gemma 3
  model built specifically for translation (55 languages):

  ```bash
  ollama pull translategemma
  ```

  Any Ollama model works via `--model` (e.g. `llama3.2:3b`, `mistral`).
- A directory of locale JSON files

## Build & install

```bash
go build -o ollama-i18n .
# optionally: go install github.com/farizfadian/go-ollama-i18n@latest
```

## Usage

Translate every existing locale in a directory (missing keys only):

```bash
ollama-i18n -s en -d ./locales
```

Translate to a specific language, creating the file if it doesn't exist:

```bash
ollama-i18n -s en -t id -d ./locales
```

Use a different model and re-translate everything (ignore the cache):

```bash
ollama-i18n -s en -d ./locales -m mistral --no-cache
```

Preview what would change without calling Ollama or writing files:

```bash
ollama-i18n -s en -d ./locales --dry-run
```

## Options

| Flag                 | Default                  | Description                                                        |
| -------------------- | ------------------------ | ------------------------------------------------------------------ |
| `-d, --dir`          | —                        | Directory containing locale files (required)                       |
| `-s, --source`       | —                        | Source locale name without extension, e.g. `en` (required)         |
| `-t, --target`       | —                        | Target locale; if omitted, all other locales in `--dir` are done   |
| `-m, --model`        | `translategemma`         | Ollama model to use                                                |
| `--host`             | `http://localhost:11434` | Ollama base URL (or set `OLLAMA_HOST`)                             |
| `--concurrency`      | `4`                      | Concurrent translation requests                                    |
| `--timeout`          | `120s`                   | Per-request timeout                                                |
| `--no-cache`         | `false`                  | Retranslate every key, ignoring existing translations             |
| `--dry-run`          | `false`                  | Report changes without calling Ollama or writing files            |
| `-v, --version`      | —                        | Print version                                                      |

The source language (from `--source`) is passed to the model along with the
target, since TranslateGemma is tuned for an explicit "{source} to {target}"
prompt. This is harmless for general-purpose models.

## Locale file structure

```
locales/
  en.json   # source
  de.json
  id.json
```

```json
{
  "common": { "save": "Save", "cancel": "Cancel" },
  "validation": {
    "required": "{field} is required",
    "minLength": "{field} must be at least {min} characters"
  }
}
```

Placeholders like `{field}`, `{{count}}`, `%s`, `:id` are preserved — the model
is instructed to leave them untouched.

## Pre-commit hook

`.git/hooks/pre-commit`:

```bash
#!/bin/sh
ollama-i18n -s en -d ./locales || exit 1
git add locales/*.json
```

## Notes & limitations

- **Model must be pulled first.** `translategemma` defaults to the 4B variant
  (~3.3 GB). Larger `translategemma:12b` / `:27b` exist if you have the VRAM.
- **Ollama concurrency:** `--concurrency` only speeds things up if your Ollama
  is configured to serve parallel requests (`OLLAMA_NUM_PARALLEL`). Against a
  single-slot instance the requests just queue — no harm, no speedup.
- **Arrays / numbers / booleans** are copied through unchanged, not translated.
- **Verify placeholders.** Translation-specialized models can occasionally
  reformat or drop placeholders like `{field}`. The prompt instructs the model
  to keep them, but review diffs before committing — especially on a new model.
- Empty source files are valid (treated as an empty locale).

## Layout

```
ordered.go     order-preserving JSON object (load/save)
provider.go    Provider interface + Ollama client
translate.go   merge/cache/walk logic + bounded concurrency
main.go        CLI: flags, file discovery, orchestration
main_test.go   tests (ordering, cache, placeholders, concurrency)
```

## Credits

Port of [`fkapsahili/ollama-i18n`](https://github.com/fkapsahili/ollama-i18n).

## License

MIT.

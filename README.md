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

### Trying it safely on a real project

The tool rewrites locale files in place, so let git be the safety net:

```bash
git status                                  # commit or stash locale changes first
ollama-i18n -s en -d ./locales --dry-run    # see the scope before any model call
ollama-i18n -s en -d ./locales              # run for real
git diff ./locales                          # review before committing
```

Each line of output ends with a count worth reading:

```
id      → Indonesian           wrote  (translated 8, kept 0, copied 2)
```

- `translated` — strings sent to the model this run
- `kept` — existing translations left untouched (the cache working)
- `copied` — non-string values (numbers, booleans, arrays) passed through

Running the same command twice should report `translated 0` the second time.
If it doesn't, something is defeating the cache — check that the target file
really has non-empty strings for those keys.

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

Placeholders like `{field}`, `{{count}}`, `%s`, `{0}`, and `<0>` tags are
masked as neutral `[[n]]` markers before translation and restored afterwards,
so the model never translates the text inside them.

Output is written with HTML escaping disabled, so markup stays readable in the
file (`"Click <0>here</0>"`, not `"Click \u003c0\u003ehere"`).

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
- **Placeholder masking.** Translation-tuned models like TranslateGemma will
  otherwise translate the words inside `{field}` (turning it into `{bidang}`).
  The tool masks placeholders as neutral `[[0]]` markers before sending and
  restores them afterwards, which survives translation reliably. Still worth a
  quick diff review on a new model, in case one drops a marker.
- Empty source files are valid (treated as an empty locale).

## Development

```bash
go test ./...          # unit tests (no Ollama required — they use a fake provider)
go vet ./...
gofmt -l .             # should print nothing
```

The tests cover key ordering, the translation cache, placeholder masking,
BOM handling, HTML-escape-free output, and concurrent merges. None of them
call Ollama, so they run in milliseconds.

## Layout

```
ordered.go       order-preserving JSON object + escape-free marshaling
provider.go      Provider interface + Ollama client
placeholder.go   mask/restore of {placeholders} as [[n]] markers
translate.go     language names + merge/cache/walk logic + bounded concurrency
main.go          CLI: flags, file discovery, orchestration, locale read/write

main_test.go         ordering, cache, placeholders reaching the provider, concurrency
placeholder_test.go  mask/restore round trips
loadlocale_test.go   BOM stripping, escape-free output, write/load round trip
lang_test.go         locale code → language name resolution
```

## Credits

Port of [`fkapsahili/ollama-i18n`](https://github.com/fkapsahili/ollama-i18n).

## License

MIT.

# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`go-ollama-i18n` — a CLI that fills in missing keys in i18n locale JSON files
using a local [Ollama](https://ollama.com) model. A Go port of
[`fkapsahili/ollama-i18n`](https://github.com/fkapsahili/ollama-i18n) (TypeScript).

Module: `github.com/farizfadian/go-ollama-i18n`

Flow: read `<source>.json` → walk it in order → for each string, keep the
existing translation if the target file already has one, otherwise send it to
the model → write the target file back preserving source key order.

## Commands

```bash
go build -o ollama-i18n .
go test ./...        # no Ollama needed; tests use a fake Provider
go vet ./...
gofmt -l .           # must print nothing

# manual end-to-end (requires Ollama running with translategemma pulled)
./ollama-i18n -s en -t id -d ./locales --dry-run
./ollama-i18n -s en -t id -d ./locales
```

## Hard constraints

**Zero external dependencies.** `go.mod` must contain only the stdlib. Do not
add libraries — not for JSON, not for CLI parsing, not for testing. This is a
deliberate design goal (single static binary, no supply chain).

**Tests must never require a running Ollama.** Use the fake provider pattern in
`main_test.go`. Unit tests run in milliseconds.

## Non-obvious behaviour — do not "simplify" these

These were established by probing the real model. Reverting any of them
reintroduces a silent data-corruption bug.

### 1. Placeholders must be masked, not merely prompted about

`translategemma` translates the words *inside* placeholders. Measured against
the real model with an explicit "preserve placeholders" system prompt:

| Input | Raw model output |
| --- | --- |
| `{field} is required` | `{bidang} diperlukan` |
| `Welcome to BizCore, {name}!` | `... {nama}!` |
| `{0} must be at least {1} characters` | `{0}` **dropped entirely** |

So `placeholder.go` replaces placeholders with neutral `[[0]]`, `[[1]]` markers
before the request and restores them after. `[[n]]` was chosen empirically: it
survived in every sentence position tested, while `{0}` and `%0` were dropped
when they appeared at the start of a sentence.

Prompt wording alone is **not** sufficient. Keep the masking.

### 2. `json.Marshal` re-escapes HTML — use the encoder path

`encoding/json` escapes `<`, `>` and `&` by default, so `<0>here</0>` would be
written as `\u003c0\u003ehere\u003c/0\u003e`. Valid JSON, but unreadable in a
diff and alarming to reviewers of i18next `<Trans>` strings.

Two places cooperate, and both are required:

- `writeLocale` (main.go) uses `json.NewEncoder` with `SetEscapeHTML(false)` and
  `SetIndent("", "  ")`.
- `OrderedMap.MarshalJSON` (ordered.go) uses the `marshalNoEscapeHTML` helper
  internally, not `json.Marshal` — otherwise nested objects come back
  pre-escaped and the outer encoder cannot undo it.

Note: a plain `json.Marshal(orderedMap)` from a caller *will* still escape,
because the escaping happens while compacting whatever `MarshalJSON` returned.
That is a stdlib constraint, not a bug here. Tests that assert unescaped output
must go through `marshalNoEscapeHTML` or `writeLocale`.

### 3. Key order is preserved deliberately

Go marshals `map[string]any` with keys sorted alphabetically, which would
reshuffle every locale file and produce enormous diffs. `OrderedMap` (ordered.go)
parses with the streaming decoder to retain source order. Do not replace it with
a plain map.

### 4. UTF-8 BOM must be tolerated on input

Locale files saved by Notepad or PowerShell's `Set-Content -Encoding UTF8` start
with `EF BB BF`, which makes `json.Unmarshal` fail with
`invalid character 'ï'`. `loadLocale` trims it. Output is written without a BOM.

### 5. Provider takes both source and target language

`Provider.Translate(ctx, text, sourceLang, targetLang)`. TranslateGemma is tuned
for an explicit "{source} to {target}" instruction, so the source language is
passed through from `--source`. Harmless for general-purpose models.

## Architecture

```
ordered.go       OrderedMap: order-preserving JSON object + marshalNoEscapeHTML
placeholder.go   maskPlaceholders / restorePlaceholders ([[n]] markers)
provider.go      Provider interface + OllamaProvider (HTTP /api/chat)
translate.go     languageName table + buildTree merge/cache + worker pool
main.go          flag parsing, locale discovery, loadLocale/writeLocale
```

`Provider` is an interface on purpose: a Claude or OpenAI backend can be added
without touching merge logic. `dryRunProvider` in main.go is the trivial
implementation used by `--dry-run`.

Concurrency: `Translate` collects jobs during the tree walk, runs them through a
bounded worker pool, then writes results back **sequentially** — the target
`OrderedMap` is not safe for concurrent writes. Keep that ordering.

Note that `--concurrency` only helps if Ollama is configured for parallel
requests (`OLLAMA_NUM_PARALLEL`); otherwise requests just queue.

## Conventions

- Comments explain *why*, especially around the constraints above. The empirical
  findings are the valuable part; don't strip them as "noise".
- Locale files are rewritten in place, so the tool assumes the user has git as a
  safety net. `--dry-run` exists to inspect scope first.
- Non-string values (numbers, booleans, arrays) are copied through untranslated.
  Translating array string elements is a known non-goal for v1.

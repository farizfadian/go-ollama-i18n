package main

import (
	"context"
	"strings"
	"testing"
)

// ── masking: HTML now travels as opaque markers, like {placeholders} ──

func TestHTMLTagsAreMasked(t *testing.T) {
	in := `Headers <span class="text-muted">one per line</span>`
	masked, orig := maskPlaceholders(in)
	if masked != "Headers [[0]]one per line[[1]]" {
		t.Fatalf("tags not masked: %q", masked)
	}
	if len(orig) != 2 || orig[0] != `<span class="text-muted">` {
		t.Fatalf("attributes not carried through: %v", orig)
	}
	if got := restorePlaceholders(masked, orig); got != in {
		t.Fatalf("round trip failed:\n in:  %q\n got: %q", in, got)
	}
}

// A model that translated `class="text-muted"` would produce markup that no
// longer matches the stylesheet — the failure this masking exists to prevent.
func TestAttributeValuesNeverReachTheModel(t *testing.T) {
	masked, _ := maskPlaceholders(`<a href="https://x.dev" title="Open docs">Docs</a>`)
	for _, leaked := range []string{"href", "https://x.dev", "Open docs", "title"} {
		if strings.Contains(masked, leaked) {
			t.Errorf("%q survived masking: %q", leaked, masked)
		}
	}
	if !strings.Contains(masked, "Docs") {
		t.Errorf("element text should still be translatable: %q", masked)
	}
}

// <code> contents are literal: `Name: value` is an example to copy, not prose.
func TestLiteralElementMaskedWithItsContents(t *testing.T) {
	masked, orig := maskPlaceholders("Headers — one per line, <code>Name: value</code>")
	if strings.Contains(masked, "Name: value") {
		t.Fatalf("code contents reached the model: %q", masked)
	}
	if len(orig) != 1 || orig[0] != "<code>Name: value</code>" {
		t.Fatalf("code element not masked as one unit: %v", orig)
	}
}

func TestNestedPlaceholderInsideLiteralElement(t *testing.T) {
	in := "Replaced with <code>{prompt}</code> at run time."
	masked, orig := maskPlaceholders(in)
	if strings.Contains(masked, "{prompt}") {
		t.Fatalf("placeholder inside <code> leaked: %q", masked)
	}
	if got := restorePlaceholders(masked, orig); got != in {
		t.Fatalf("round trip failed:\n in:  %q\n got: %q", in, got)
	}
}

// Plain prose must be left alone — masking everything would defeat the point.
func TestComparisonOperatorIsNotMistakenForATag(t *testing.T) {
	masked, orig := maskPlaceholders("Fails when a < b and b > c")
	if len(orig) != 0 {
		t.Fatalf("plain text was masked: %q -> %v", masked, orig)
	}
}

// ── leak detection: the model answering instead of translating ──

func TestLeakReasonCatchesObservedFailures(t *testing.T) {
	cases := []struct {
		name, src, got string
		want           bool
	}{
		// Real translategemma output for the source string "Polish": it read the
		// word as the language and restated its own instructions, in Polish.
		{"echoed rules", "Polish", "Zasady:\n- Wyświetl tylko tłumaczenie.", true},
		{"empty", "Save", "   ", true},
		{"paragraph for a label", "Save", strings.Repeat("una explicación larga ", 6), true},
		{"good short label", "Save", "Guardar", false},
		{"legitimate expansion", "Undo", "Rückgängig machen", false},
		{"multi-line source stays allowed", "a\nb", "x\ny", false},
	}
	for _, c := range cases {
		if got := leakReason(c.src, c.got, 0) != ""; got != c.want {
			t.Errorf("%s: leak=%v want %v (reason %q)", c.name, got, c.want, leakReason(c.src, c.got, 0))
		}
	}
}

func TestLeakReasonCatchesLostMarkers(t *testing.T) {
	if leakReason("[[0]] items", "elementos", 1) == "" {
		t.Error("a reply that dropped its marker should be rejected")
	}
	if r := leakReason("[[0]] items", "[[0]] elementos", 1); r != "" {
		t.Errorf("a faithful reply was rejected: %s", r)
	}
}

// ── end to end: a leak must not abort the run or reach the file ──

type leakyProvider struct{ fail map[string]bool }

func (leakyProvider) Name() string { return "leaky" }
func (p leakyProvider) Translate(_ context.Context, req Request) (string, error) {
	if p.fail[req.Text] {
		return "", ErrLeaked
	}
	return "T(" + req.Text + ")", nil
}

func TestLeakLeavesTheKeyOutSoTheNextRunRetriesIt(t *testing.T) {
	src := mustLoad(t, `{"a":"Save","b":"Polish","c":"Cancel"}`)
	p := leakyProvider{fail: map[string]bool{"Polish": true}}

	out, stats, err := Translate(context.Background(), p, src, NewOrderedMap(), "English", "Spanish", false, 1)
	if err != nil {
		t.Fatalf("one bad reply aborted the whole locale: %v", err)
	}
	// Writing the source string here would look like a translation to every
	// later run ("kept"), to any parity check, and to a reader of the file:
	// the key would be in the source language forever. Left out, it is a gap —
	// the app falls back exactly as it does for a key nobody has translated
	// yet, and the next run tries again.
	if v, ok := out.Get("b"); ok {
		t.Errorf("rejected key written to the file as %q; it should be left out", v)
	}
	if v, _ := out.Get("a"); v != "T(Save)" {
		t.Errorf("neighbouring keys should be unaffected: %v", v)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Translated != 2 {
		t.Errorf("Translated = %d, want 2 (the rejected one does not count)", stats.Translated)
	}

	// The next run, with a model that behaves, fills it in and keeps the rest.
	again, stats, err := Translate(context.Background(), leakyProvider{}, src, out, "English", "Spanish", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := again.Get("b"); v != "T(Polish)" {
		t.Errorf("the next run did not retry the rejected key: %v", v)
	}
	if stats.Kept != 2 || stats.Translated != 1 {
		t.Errorf("next run: kept %d, translated %d; want 2 and 1", stats.Kept, stats.Translated)
	}
}

// The key is what lets a model tell the verb "Polish" from the language.
func TestKeyReachesTheProvider(t *testing.T) {
	src := mustLoad(t, `{"aichat_page":{"polish":"Polish"}}`)
	fp := &fakeProvider{}
	if _, _, err := Translate(context.Background(), fp, src, NewOrderedMap(), "English", "Spanish", false, 1); err != nil {
		t.Fatal(err)
	}
	if len(fp.keys) != 1 || fp.keys[0] != "aichat_page.polish" {
		t.Fatalf("provider got keys %v, want [aichat_page.polish]", fp.keys)
	}
}

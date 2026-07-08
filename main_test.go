package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeProvider records what it was asked to translate and returns a
// deterministic, placeholder-preserving output.
type fakeProvider struct{ seen []string }

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Translate(_ context.Context, text, sourceLang, targetLang string) (string, error) {
	f.seen = append(f.seen, text)
	return "T(" + text + ")", nil
}

func mustLoad(t *testing.T, s string) *OrderedMap {
	t.Helper()
	m := NewOrderedMap()
	if err := json.Unmarshal([]byte(s), m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestOrderPreservedOnRoundTrip(t *testing.T) {
	in := `{"zebra":"1","apple":"2","nested":{"mango":"3","banana":"4"}}`
	m := mustLoad(t, in)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("order not preserved:\n got: %s\nwant: %s", out, in)
	}
}

func TestCacheKeepsExistingAndTranslatesMissing(t *testing.T) {
	src := mustLoad(t, `{"common":{"save":"Save","cancel":"Cancel"},"hi":"Hello"}`)
	existing := mustLoad(t, `{"common":{"save":"Simpan"}}`)
	fp := &fakeProvider{}

	out, stats, err := Translate(context.Background(), fp, src, existing, "English", "Indonesian", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Kept != 1 || stats.Translated != 2 {
		t.Fatalf("stats = %+v, want Kept 1 / Translated 2", stats)
	}

	got, _ := json.Marshal(out)
	want := `{"common":{"save":"Simpan","cancel":"T(Cancel)"},"hi":"T(Hello)"}`
	if string(got) != want {
		t.Fatalf("merge wrong:\n got: %s\nwant: %s", got, want)
	}
}

func TestNoCacheRetranslatesEverything(t *testing.T) {
	src := mustLoad(t, `{"save":"Save"}`)
	existing := mustLoad(t, `{"save":"Simpan"}`)
	fp := &fakeProvider{}

	_, stats, err := Translate(context.Background(), fp, src, existing, "English", "Indonesian", true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Translated != 1 || stats.Kept != 0 {
		t.Fatalf("stats = %+v, want Translated 1 / Kept 0", stats)
	}
}

func TestPlaceholdersReachProviderIntact(t *testing.T) {
	src := mustLoad(t, `{"msg":"{field} must be at least {min} characters"}`)
	fp := &fakeProvider{}
	if _, _, err := Translate(context.Background(), fp, src, NewOrderedMap(), "English", "French", false, 1); err != nil {
		t.Fatal(err)
	}
	if len(fp.seen) != 1 || !strings.Contains(fp.seen[0], "{field}") || !strings.Contains(fp.seen[0], "{min}") {
		t.Fatalf("placeholders mangled before provider: %v", fp.seen)
	}
}

func TestNonStringValuesCopiedThrough(t *testing.T) {
	src := mustLoad(t, `{"count":5,"enabled":true,"tags":["a","b"]}`)
	out, stats, err := Translate(context.Background(), &fakeProvider{}, src, NewOrderedMap(), "English", "German", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Copied != 3 || stats.Translated != 0 {
		t.Fatalf("stats = %+v, want Copied 3 / Translated 0", stats)
	}
	got, _ := json.Marshal(out)
	if string(got) != `{"count":5,"enabled":true,"tags":["a","b"]}` {
		t.Fatalf("non-string passthrough wrong: %s", got)
	}
}

func TestConcurrencyKeepsOrderAndCorrectness(t *testing.T) {
	src := mustLoad(t, `{"a":"A","b":"B","c":"C","d":"D","e":"E"}`)
	out, _, err := Translate(context.Background(), &fakeProvider{}, src, NewOrderedMap(), "English", "Spanish", false, 8)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(out)
	want := `{"a":"T(A)","b":"T(B)","c":"T(C)","d":"T(D)","e":"T(E)"}`
	if string(got) != want {
		t.Fatalf("concurrent merge wrong:\n got: %s\nwant: %s", got, want)
	}
}

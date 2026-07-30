package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A UTF-8 BOM at the start of a locale file (common when files are saved by
// Notepad or PowerShell on Windows) must not break JSON parsing.
func TestLoadLocaleStripsBOM(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "en.json")
	withBOM := append([]byte("\xef\xbb\xbf"), []byte(`{"a":"b"}`)...)
	if err := os.WriteFile(p, withBOM, 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadLocale(p)
	if err != nil {
		t.Fatalf("loadLocale with BOM failed: %v", err)
	}
	if v, _ := m.Get("a"); v != "b" {
		t.Fatalf("got %v, want %q", v, "b")
	}
}

// encoding/json escapes <, > and & by default, turning "<0>here</0>" into
// "\u003c0\u003ehere\u003c/0\u003e". That is valid JSON but unreadable in a
// diff, so writeLocale must disable HTML escaping.
func TestWriteLocaleDoesNotEscapeHTML(t *testing.T) {
	src := mustLoad(t, `{"trans":"Click <0>here</0> to continue","legal":"Terms & Conditions"}`)

	dir := t.TempDir()
	p := filepath.Join(dir, "id.json")
	if err := writeLocale(p, src); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	if strings.Contains(got, `\u003c`) || strings.Contains(got, `\u003e`) || strings.Contains(got, `\u0026`) {
		t.Fatalf("output is HTML-escaped:\n%s", got)
	}
	if !strings.Contains(got, "<0>here</0>") {
		t.Fatalf("markup not written literally:\n%s", got)
	}
	if !strings.Contains(got, "Terms & Conditions") {
		t.Fatalf("ampersand not written literally:\n%s", got)
	}
	if !strings.HasSuffix(got, "}\n") {
		t.Fatalf("file should end with a single newline, got %q", got[len(got)-3:])
	}
}

// The unescaped output must still be valid JSON that parses back identically.
//
// Note: the comparison goes through marshalNoEscapeHTML rather than
// json.Marshal. json.Marshal re-escapes <, > and & while compacting whatever
// MarshalJSON returns, so only the writing path (which uses an Encoder with
// SetEscapeHTML(false)) can produce literal markup — see writeLocale.
func TestWriteLocaleRoundTripsThroughLoad(t *testing.T) {
	in := `{"trans":"Click <0>here</0> to continue","legal":"Terms & Conditions","n":5,"ok":true}`
	src := mustLoad(t, in)

	dir := t.TempDir()
	p := filepath.Join(dir, "id.json")
	if err := writeLocale(p, src); err != nil {
		t.Fatal(err)
	}
	back, err := loadLocale(p)
	if err != nil {
		t.Fatalf("written file does not parse: %v", err)
	}
	got, err := marshalNoEscapeHTML(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != in {
		t.Fatalf("round trip changed content:\n got: %s\nwant: %s", got, in)
	}
}

// Nested objects are marshaled through OrderedMap.MarshalJSON, so markup in a
// sub-object must survive that path unescaped too.
func TestNestedMarkupSurvivesMarshalJSON(t *testing.T) {
	m := mustLoad(t, `{"outer":{"inner":"a <b> & c"}}`)
	got, err := marshalNoEscapeHTML(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"outer":{"inner":"a <b> & c"}}` {
		t.Fatalf("nested markup escaped: %s", got)
	}
}

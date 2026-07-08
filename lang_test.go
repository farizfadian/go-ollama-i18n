package main

import "testing"

func TestLanguageName(t *testing.T) {
	cases := map[string]string{
		"en":      "English",              // base subtag
		"id":      "Indonesian",           // base subtag
		"sw":      "Swahili",              // newly added
		"en-US":   "American English",     // full-tag variant wins
		"pt-BR":   "Brazilian Portuguese", // full-tag variant wins
		"pt_BR":   "Brazilian Portuguese", // underscore normalized to hyphen
		"zh-Hant": "Traditional Chinese",  // script variant
		"fr-CA":   "Canadian French",      // region variant
		"de-CH":   "German",               // no de-CH entry -> base "de"
		"zz":      "zz",                   // unknown -> raw code
	}
	for code, want := range cases {
		if got := languageName(code); got != want {
			t.Errorf("languageName(%q) = %q, want %q", code, got, want)
		}
	}
}

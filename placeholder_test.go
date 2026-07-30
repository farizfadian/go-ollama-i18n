package main

import "testing"

func TestMaskRestoreRoundTrip(t *testing.T) {
	cases := []string{
		"{field} is required",
		"{field} must be at least {min} characters",
		"Welcome to BizCore, {name}!",
		"You have {{count}} new messages",
		"Delete {0} of {1} items?",
		"Click <0>here</0> to continue",
		"Loaded %s in %d ms",
		"Position %1$s of %2$s",
		"No placeholders here",
		"",
	}
	for _, in := range cases {
		masked, orig := maskPlaceholders(in)
		if got := restorePlaceholders(masked, orig); got != in {
			t.Errorf("round trip failed:\n in:  %q\n got: %q", in, got)
		}
	}
}

func TestMaskProducesNeutralMarkers(t *testing.T) {
	masked, orig := maskPlaceholders("{field} must be at least {min} characters")
	if masked != "[[0]] must be at least [[1]] characters" {
		t.Fatalf("unexpected mask: %q", masked)
	}
	if len(orig) != 2 || orig[0] != "{field}" || orig[1] != "{min}" {
		t.Fatalf("unexpected originals: %v", orig)
	}
}

// Simulates the real translategemma behaviour observed in testing: it translates
// the surrounding words but leaves [[n]] markers untouched.
func TestRestoreSurvivesModelTranslatingAround(t *testing.T) {
	_, orig := maskPlaceholders("{field} must be at least {min} characters")
	modelOut := "[[0]] harus setidaknya [[1]] karakter"
	got := restorePlaceholders(modelOut, orig)
	want := "{field} harus setidaknya {min} karakter"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDoubleBraceMaskedAsWhole(t *testing.T) {
	masked, orig := maskPlaceholders("You have {{count}} messages")
	if masked != "You have [[0]] messages" {
		t.Fatalf("double brace not masked as a unit: %q", masked)
	}
	if len(orig) != 1 || orig[0] != "{{count}}" {
		t.Fatalf("unexpected originals: %v", orig)
	}
}

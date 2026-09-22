package main

import (
	"fmt"
	"regexp"
	"strings"
)

// literalElements hold code, not prose. Their *contents* must survive
// translation untouched, so the whole element is masked as one unit rather than
// just its tags — otherwise `<code>Name: value</code>` comes back as
// `<code>Nombre: valor</code>` and the example stops being copy-pastable.
var literalElements = []string{"code", "kbd", "samp", "var", "pre"}

// placeholderRe matches everything that must reach the model as an opaque
// marker rather than as words:
//
//	<code>x</code>  literal elements, contents included
//	<span class=…>  HTML tags and their attributes
//	<0> </0>        i18next <Trans> tags
//	{{count}}       double-brace (i18next, Vue, Handlebars)
//	{name} {0}      single-brace (ICU, Angular, .NET composite formatting)
//	%s %d %1$s      printf-style
//
// Matched fragments are swapped for neutral [[n]] markers before a string is
// sent to the model. Translation-tuned models (e.g. translategemma) otherwise
// translate the words *inside* them — {field} becomes {bidang}, and
// `class="text-muted"` becomes `class="texto-apagado"` — which silently breaks
// runtime interpolation and layout. Neutral markers have nothing to translate,
// so they survive and are restored verbatim afterwards.
//
// Ordering matters: literal elements come first so that leftmost-first
// alternation consumes the whole element before the bare-tag branch can match
// only its opening tag.
var placeholderRe = regexp.MustCompile(`(?is)` + strings.Join(append(
	literalElementPatterns(),
	`</?[a-z][^<>]*>`, // any other HTML/XML tag, attributes included
	`</?\d+>`,         // i18next <Trans> numbered tags
	`\{\{[^{}]*\}\}`,  // {{count}}
	`\{[^{}]*\}`,      // {name}, {0}
	`%\d+\$[a-z]`,     // %1$s
	`%[a-z]`,          // %s, %d
), "|"))

// literalElementPatterns builds one alternative per literal element. RE2 has no
// backreferences, so `<(code|kbd)\b[^>]*>.*?</\1>` is not available and each
// element has to be spelled out.
func literalElementPatterns() []string {
	pats := make([]string, 0, len(literalElements))
	for _, e := range literalElements {
		pats = append(pats, `<`+e+`\b[^>]*>.*?</`+e+`>`)
	}
	return pats
}

// maskPlaceholders replaces every placeholder with a [[n]] marker and returns
// the masked string plus the originals, indexed by marker number.
func maskPlaceholders(s string) (string, []string) {
	var originals []string
	masked := placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		i := len(originals)
		originals = append(originals, m)
		return fmt.Sprintf("[[%d]]", i)
	})
	return masked, originals
}

// restorePlaceholders swaps [[n]] markers back for their originals. The trailing
// `]]` makes markers unambiguous, so [[1]] never matches inside [[10]].
func restorePlaceholders(s string, originals []string) string {
	for i, orig := range originals {
		s = strings.ReplaceAll(s, fmt.Sprintf("[[%d]]", i), orig)
	}
	return s
}

// markerCount reports how many [[n]] markers a string still carries. A reply
// that dropped or invented markers cannot be restored faithfully, so the caller
// treats it as a failed translation rather than writing it out.
var markerRe = regexp.MustCompile(`\[\[\d+\]\]`)

func markerCount(s string) int { return len(markerRe.FindAllString(s, -1)) }

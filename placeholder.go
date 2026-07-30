package main

import (
	"fmt"
	"regexp"
	"strings"
)

// placeholderRe matches the interpolation styles commonly found in i18n files:
//
//	{{count}}      double-brace (i18next, Vue, Handlebars)
//	{name} {0}     single-brace (ICU, Angular, .NET composite formatting)
//	<0> </0>       i18next <Trans> tags
//	%s %d %1$s     printf-style
//
// Matched fragments are swapped for neutral [[n]] markers before a string is
// sent to the model. Translation-tuned models (e.g. translategemma) otherwise
// translate the words *inside* placeholders — {field} becomes {bidang} — which
// silently breaks runtime interpolation. Neutral markers have nothing to
// translate, so they survive and are restored verbatim afterwards.
var placeholderRe = regexp.MustCompile(`\{\{[^{}]*\}\}|\{[^{}]*\}|</?\d+>|%\d+\$[A-Za-z]|%[A-Za-z]`)

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

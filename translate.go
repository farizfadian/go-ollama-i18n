package main

import (
	"context"
	"strings"
	"sync"
)

// languageNames maps locale codes to human-readable names that read better in
// the model prompt than a bare code. Lookup tries the full normalized tag first
// (so "pt-BR" -> "Brazilian Portuguese"), then falls back to the base language
// subtag ("pt" -> "Portuguese"), and finally returns the raw code if unknown.
var languageNames = map[string]string{
	// Base ISO 639-1 language subtags.
	"af": "Afrikaans", "sq": "Albanian", "am": "Amharic", "ar": "Arabic",
	"hy": "Armenian", "as": "Assamese", "az": "Azerbaijani", "eu": "Basque",
	"be": "Belarusian", "bn": "Bengali", "bs": "Bosnian", "bg": "Bulgarian",
	"my": "Burmese", "ca": "Catalan", "ceb": "Cebuano", "zh": "Chinese",
	"co": "Corsican", "hr": "Croatian", "cs": "Czech", "da": "Danish",
	"nl": "Dutch", "en": "English", "eo": "Esperanto", "et": "Estonian",
	"fil": "Filipino", "fi": "Finnish", "fr": "French", "fy": "Frisian",
	"gl": "Galician", "ka": "Georgian", "de": "German", "el": "Greek",
	"gu": "Gujarati", "ht": "Haitian Creole", "ha": "Hausa", "haw": "Hawaiian",
	"he": "Hebrew", "iw": "Hebrew", "hi": "Hindi", "hmn": "Hmong",
	"hu": "Hungarian", "is": "Icelandic", "ig": "Igbo", "id": "Indonesian",
	"ga": "Irish", "it": "Italian", "ja": "Japanese", "jv": "Javanese",
	"kn": "Kannada", "kk": "Kazakh", "km": "Khmer", "rw": "Kinyarwanda",
	"ko": "Korean", "ku": "Kurdish", "ky": "Kyrgyz", "lo": "Lao",
	"la": "Latin", "lv": "Latvian", "lt": "Lithuanian", "lb": "Luxembourgish",
	"mk": "Macedonian", "mg": "Malagasy", "ms": "Malay", "ml": "Malayalam",
	"mt": "Maltese", "mi": "Maori", "mr": "Marathi", "mn": "Mongolian",
	"ne": "Nepali", "no": "Norwegian", "nb": "Norwegian Bokmal",
	"nn": "Norwegian Nynorsk", "ny": "Nyanja", "or": "Odia", "ps": "Pashto",
	"fa": "Persian", "pl": "Polish", "pt": "Portuguese", "pa": "Punjabi",
	"ro": "Romanian", "ru": "Russian", "sm": "Samoan", "gd": "Scottish Gaelic",
	"sr": "Serbian", "st": "Sesotho", "sn": "Shona", "sd": "Sindhi",
	"si": "Sinhala", "sk": "Slovak", "sl": "Slovenian", "so": "Somali",
	"es": "Spanish", "su": "Sundanese", "sw": "Swahili", "sv": "Swedish",
	"tg": "Tajik", "ta": "Tamil", "tt": "Tatar", "te": "Telugu",
	"th": "Thai", "tl": "Tagalog", "tr": "Turkish", "tk": "Turkmen",
	"uk": "Ukrainian", "ur": "Urdu", "ug": "Uyghur", "uz": "Uzbek",
	"vi": "Vietnamese", "cy": "Welsh", "xh": "Xhosa", "yi": "Yiddish",
	"yo": "Yoruba", "zu": "Zulu",

	// Common region/script variants worth distinguishing for translation.
	"pt-br":   "Brazilian Portuguese",
	"pt-pt":   "European Portuguese",
	"zh-hans": "Simplified Chinese",
	"zh-hant": "Traditional Chinese",
	"zh-cn":   "Simplified Chinese",
	"zh-sg":   "Simplified Chinese",
	"zh-tw":   "Traditional Chinese",
	"zh-hk":   "Traditional Chinese (Hong Kong)",
	"en-gb":   "British English",
	"en-us":   "American English",
	"en-au":   "Australian English",
	"es-419":  "Latin American Spanish",
	"es-mx":   "Mexican Spanish",
	"es-es":   "European Spanish",
	"fr-ca":   "Canadian French",
	"fr-fr":   "European French",
}

// languageName resolves a locale code to a human-readable language name.
func languageName(code string) string {
	norm := strings.ToLower(strings.ReplaceAll(code, "_", "-"))
	// Try the full tag first so region/script variants win (e.g. "pt-br").
	if name, ok := languageNames[norm]; ok {
		return name
	}
	// Fall back to the base language subtag (e.g. "pt-br" -> "pt").
	base := norm
	if i := strings.IndexByte(base, '-'); i >= 0 {
		base = base[:i]
	}
	if name, ok := languageNames[base]; ok {
		return name
	}
	return code
}

type Stats struct {
	Translated int // strings sent to the provider
	Kept       int // existing translations preserved (cache hit)
	Copied     int // non-string values copied through unchanged
}

// job records one string that needs translating, plus where to write it back.
type job struct {
	target *OrderedMap
	key    string
	text   string
}

// buildTree produces the output object in source order. Existing non-empty
// string translations are kept unless noCache is set; everything else is
// queued for translation. Returned jobs are filled in by the caller.
func buildTree(src, existing *OrderedMap, noCache bool, jobs *[]job, stats *Stats) *OrderedMap {
	out := NewOrderedMap()
	for _, key := range src.Keys() {
		srcVal, _ := src.Get(key)

		switch v := srcVal.(type) {
		case *OrderedMap:
			var existingSub *OrderedMap
			if existing != nil {
				if ev, ok := existing.Get(key); ok {
					if em, ok := ev.(*OrderedMap); ok {
						existingSub = em
					}
				}
			}
			out.Set(key, buildTree(v, existingSub, noCache, jobs, stats))

		case string:
			if !noCache && existing != nil {
				if ev, ok := existing.Get(key); ok {
					if es, ok := ev.(string); ok && strings.TrimSpace(es) != "" {
						out.Set(key, es)
						stats.Kept++
						continue
					}
				}
			}
			out.Set(key, "") // placeholder fixes ordering before async fill
			*jobs = append(*jobs, job{target: out, key: key, text: v})
			stats.Translated++

		default:
			// numbers, bools, null, arrays: keep existing if present, else copy.
			if existing != nil {
				if ev, ok := existing.Get(key); ok {
					out.Set(key, ev)
					continue
				}
			}
			out.Set(key, v)
			stats.Copied++
		}
	}
	return out
}

// Translate builds the merged tree for one target locale and runs all pending
// translation jobs with bounded concurrency.
func Translate(ctx context.Context, p Provider, src, existing *OrderedMap, sourceLang, targetLang string, noCache bool, concurrency int) (*OrderedMap, Stats, error) {
	var stats Stats
	var jobs []job
	out := buildTree(src, existing, noCache, &jobs, &stats)

	if len(jobs) == 0 {
		return out, stats, nil
	}
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]string, len(jobs))
	errs := make([]error, len(jobs))

	indices := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indices {
				out, err := p.Translate(ctx, jobs[i].text, sourceLang, targetLang)
				results[i], errs[i] = out, err
			}
		}()
	}
	for i := range jobs {
		indices <- i
	}
	close(indices)
	wg.Wait()

	// Write results back sequentially to avoid concurrent map writes.
	for i, jb := range jobs {
		if errs[i] != nil {
			return out, stats, errs[i]
		}
		jb.target.Set(jb.key, results[i])
	}
	return out, stats, nil
}

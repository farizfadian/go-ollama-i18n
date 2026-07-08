package main

import (
	"context"
	"strings"
	"sync"
)

// languageNames maps common locale codes to human-readable names that read
// better in the model prompt than a bare code. Region suffixes (en-US) are
// stripped before lookup; unknown codes fall back to the raw string.
var languageNames = map[string]string{
	"en": "English", "id": "Indonesian", "ms": "Malay", "fr": "French",
	"de": "German", "es": "Spanish", "pt": "Portuguese", "it": "Italian",
	"nl": "Dutch", "ja": "Japanese", "ko": "Korean", "zh": "Chinese",
	"th": "Thai", "vi": "Vietnamese", "ru": "Russian", "ar": "Arabic",
	"hi": "Hindi", "tr": "Turkish", "pl": "Polish", "sv": "Swedish",
	"da": "Danish", "fi": "Finnish", "no": "Norwegian", "cs": "Czech",
	"uk": "Ukrainian", "ro": "Romanian", "hu": "Hungarian", "el": "Greek",
}

func languageName(code string) string {
	base := strings.ToLower(code)
	if i := strings.IndexAny(base, "-_"); i >= 0 {
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
func Translate(ctx context.Context, p Provider, src, existing *OrderedMap, targetLang string, noCache bool, concurrency int) (*OrderedMap, Stats, error) {
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
				out, err := p.Translate(ctx, jobs[i].text, targetLang)
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

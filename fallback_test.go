package main

import (
	"context"
	"errors"
	"testing"
)

// scriptedProvider answers from a table; anything not in it is a leak, which
// is what a small model does with a bare imperative or a {{placeholder}}.
type scriptedProvider struct {
	name    string
	answers map[string]string
	calls   int
}

func (s *scriptedProvider) Name() string { return s.name }
func (s *scriptedProvider) Translate(_ context.Context, req Request) (string, error) {
	s.calls++
	if out, ok := s.answers[req.Text]; ok {
		return out, nil
	}
	return "", ErrLeaked
}

type erroringProvider struct{ err error }

func (e erroringProvider) Name() string                                       { return "boom" }
func (e erroringProvider) Translate(context.Context, Request) (string, error) { return "", e.err }

func TestFallbackAnswersOnlyWhatTheFirstModelRefused(t *testing.T) {
	first := &scriptedProvider{name: "a", answers: map[string]string{"Save": "A(Save)"}}
	second := &scriptedProvider{name: "b", answers: map[string]string{"Save": "B(Save)", "Polish": "B(Polish)"}}
	p := WithFallback(first, second)

	out, err := p.Translate(context.Background(), Request{Text: "Save"})
	if err != nil || out != "A(Save)" {
		t.Fatalf("the first model's answer should stand: %q, %v", out, err)
	}
	if second.calls != 0 {
		t.Errorf("second model was asked although the first answered")
	}

	out, err = p.Translate(context.Background(), Request{Text: "Polish"})
	if err != nil || out != "B(Polish)" {
		t.Fatalf("the fallback should answer a key the first model refused: %q, %v", out, err)
	}
	if p.Rescued() != 1 {
		t.Errorf("Rescued = %d, want 1", p.Rescued())
	}
}

func TestFallbackLeakingTooIsStillALeak(t *testing.T) {
	p := WithFallback(&scriptedProvider{name: "a"}, &scriptedProvider{name: "b"})
	if _, err := p.Translate(context.Background(), Request{Text: "Polish"}); !errors.Is(err, ErrLeaked) {
		t.Fatalf("both refusing must read as a leak, got %v", err)
	}
	if p.Rescued() != 0 {
		t.Errorf("nothing was rescued, Rescued = %d", p.Rescued())
	}

	// And through the whole run the key is left out, exactly as with one model.
	src := mustLoad(t, `{"a":"Polish","b":"Save"}`)
	p = WithFallback(&scriptedProvider{name: "a"}, &scriptedProvider{name: "b", answers: map[string]string{"Save": "B(Save)"}})
	out, stats, err := Translate(context.Background(), p, src, NewOrderedMap(), "English", "Spanish", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := out.Get("a"); ok {
		t.Errorf("key both models refused was written as %q", v)
	}
	if v, _ := out.Get("b"); v != "B(Save)" {
		t.Errorf("key the fallback answered should be written: %v", v)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
}

func TestFallbackRealErrorsSurfaceFromEitherModel(t *testing.T) {
	boom := errors.New("connection refused")

	second := &scriptedProvider{name: "b", answers: map[string]string{"Polish": "B(Polish)"}}
	p := WithFallback(erroringProvider{err: boom}, second)
	if _, err := p.Translate(context.Background(), Request{Text: "Polish"}); !errors.Is(err, boom) {
		t.Fatalf("a real error from the first model is not a leak to fall back on, got %v", err)
	}
	if second.calls != 0 {
		t.Errorf("fallback asked after a real error from the first model")
	}

	p = WithFallback(&scriptedProvider{name: "a"}, erroringProvider{err: boom})
	if _, err := p.Translate(context.Background(), Request{Text: "Polish"}); !errors.Is(err, boom) {
		t.Fatalf("a real error from the fallback (model not pulled, server down) must surface, got %v", err)
	}
}

func TestFallbackNameSaysBothModels(t *testing.T) {
	p := WithFallback(&scriptedProvider{name: "translategemma"}, &scriptedProvider{name: "qwen2.5:7b"})
	if got := p.Name(); got != "translategemma, then qwen2.5:7b" {
		t.Errorf("Name = %q", got)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider translates a single short string into the target language.
//
// Keeping this as an interface means the Ollama implementation below can be
// swapped for a Claude/OpenAI-backed one without touching the merge logic.
type Provider interface {
	Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error)
	Name() string
}

const systemPromptTmpl = `You are a professional %s to %s translator specialized in software localization.
Translate the user's text.

Rules:
- Output ONLY the translated text. No quotes, no explanations, no notes.
- Preserve every placeholder exactly as-is: {name}, {{count}}, %%s, %%d, :id, <0>, </0>.
- Preserve leading/trailing whitespace, punctuation, and capitalization style.
- Do not translate brand names, code, or HTML tags.
- If the text is already in the target language, return it unchanged.`

type OllamaProvider struct {
	BaseURL string
	Model   string
	client  *http.Client
}

func NewOllamaProvider(baseURL, model string, timeout time.Duration) *OllamaProvider {
	return &OllamaProvider{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

func (p *OllamaProvider) Name() string { return "ollama:" + p.Model }

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages []ollamaMessage `json:"messages"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error,omitempty"`
}

func (p *OllamaProvider) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	// Don't waste a model call on whitespace-only or empty strings.
	if strings.TrimSpace(text) == "" {
		return text, nil
	}

	reqBody := ollamaChatRequest{
		Model:  p.Model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "system", Content: fmt.Sprintf(systemPromptTmpl, sourceLang, targetLang)},
			{Role: "user", Content: text},
		},
		Options: map[string]any{"temperature": 0},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Ollama at %s: %w", p.BaseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out ollamaChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decoding ollama response: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama error: %s", out.Error)
	}
	return cleanTranslation(out.Message.Content), nil
}

// cleanTranslation strips wrapping quotes and stray whitespace that small
// models sometimes add despite the system prompt.
func cleanTranslation(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

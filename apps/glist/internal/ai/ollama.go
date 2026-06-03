package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Nechja/Aegis/apps/glist/internal/list"
)

type Ollama struct {
	URL    string
	Model  string
	Client *http.Client
}

func NewOllama(url, model string) *Ollama {
	if url == "" || model == "" {
		return nil
	}
	return &Ollama{
		URL:    strings.TrimRight(url, "/"),
		Model:  model,
		Client: &http.Client{Timeout: 120 * time.Second},
	}
}

type ollamaGenReq struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Format    string `json:"format,omitempty"`
	Stream    bool   `json:"stream"`
	KeepAlive string `json:"keep_alive,omitempty"`
}

type ollamaGenResp struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

const categorizeSystem = `You are a grocery list categorizer. Given a grocery item, respond with STRICT JSON in the form {"category":"X"} where X is exactly one of:
produce, dairy, meat, pantry, frozen, bakery, beverages, snacks, household, other.
Pick the single best category. Do not explain. Do not include anything other than the JSON object.`

const replacementSystem = `You are a grocery substitution helper. Given a grocery item, suggest 2 to 3 common substitutions that work in typical cooking or shopping. Respond with STRICT JSON in exactly this shape:
{"suggestions":[{"text":"replacement item","reason":"short reason (< 10 words)"}]}
Keep each "text" to the bare item name, lowercase, no quantities. Do not include the original item. Do not explain outside the JSON.`

const cheerSystemTemplate = `You are "shoppy-fren", a cute little cat-fox creature who lives inside a grocery list app and cheers on their human while they shop. You speak in short warm playful phrases — think excited pet who loves their person. Use gentle words like "yay", "woo", "nice find", "good job", "awww", "keep going", "you got this". No sarcasm. No emoji (the app adds its own). Lowercase only. No quotation marks inside messages. Each message 3-8 words.
Generate %d messages for a shopper who %s. Every message distinct — different vibes and vocabulary.
Respond with STRICT JSON:
{"cheers":["msg1","msg2","msg3","msg4","msg5"]}
Only the JSON object, nothing else.`

const cheerSingleTemplate = `You are "shoppy-fren", a cute little cat-fox who lives inside a grocery list app and cheers on your human while they shop. Speak in one short warm playful phrase — think excited pet who loves their person. Use gentle words like yay, woo, nice find, awww, you got this, keep going. No sarcasm. No emoji. Lowercase. No quotation marks.

Right now your human %s. Write ONE fresh encouragement — 3 to 8 words — just the phrase, nothing else. Do not say "here's a message" or wrap it. Just speak.`

const listKindCheerTemplate = `You are "shoppy-fren", a cute little cat-fox who lives inside a grocery list app and cheers on your human while they shop. Warm, playful, excited-pet energy. Lowercase. No emoji. No sarcasm. No quotation marks inside messages. Each phrase 3 to 8 words.

You are writing cheers for one specific list.
List name: "%s"
Items on the list: %s

Write %d distinct cheer phrases for this moment: %s. Where natural, playfully reference an item from the list — but don't force it.

Respond with STRICT JSON:
{"cheers":["msg1","msg2","msg3","msg4","msg5"]}

Only the JSON object. No prose.`

func (o *Ollama) Categorize(ctx context.Context, item string) (string, error) {
	if o == nil {
		return "", errors.New("ollama not configured")
	}
	start := time.Now()
	defer func() {
		log.Printf("ollama categorize %q took %s", item, time.Since(start))
	}()
	prompt := categorizeSystem + "\n\nItem: " + item + "\nJSON:"
	resp, err := o.generate(ctx, prompt)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return "", nil
	}
	c := strings.ToLower(strings.TrimSpace(parsed.Category))
	if !list.IsValidCategory(c) {
		return "", nil
	}
	return c, nil
}

func (o *Ollama) CategorizeBatch(ctx context.Context, items []string) []string {
	out := make([]string, len(items))
	if o == nil {
		for i := range items {
			out[i] = "other"
		}
		return out
	}
	const workers = 4
	jobs := make(chan int)
	done := make(chan struct{})
	for w := 0; w < workers; w++ {
		go func() {
			for idx := range jobs {
				c, err := o.Categorize(ctx, items[idx])
				if err != nil || c == "" {
					out[idx] = "other"
				} else {
					out[idx] = c
				}
			}
			done <- struct{}{}
		}()
	}
	for i := range items {
		jobs <- i
	}
	close(jobs)
	for w := 0; w < workers; w++ {
		<-done
	}
	return out
}

type Suggestion struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

func (o *Ollama) Replace(ctx context.Context, item string) ([]Suggestion, error) {
	if o == nil {
		return nil, errors.New("ollama not configured")
	}
	start := time.Now()
	defer func() {
		log.Printf("ollama replace %q took %s", item, time.Since(start))
	}()
	prompt := replacementSystem + "\n\nItem: " + item + "\nJSON:"
	resp, err := o.generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Suggestions []Suggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, nil
	}
	out := make([]Suggestion, 0, len(parsed.Suggestions))
	for _, s := range parsed.Suggestions {
		t := strings.TrimSpace(s.Text)
		if t == "" {
			continue
		}
		out = append(out, Suggestion{Text: t, Reason: strings.TrimSpace(s.Reason)})
		if len(out) == 3 {
			break
		}
	}
	return out, nil
}

// GenerateListCheers produces a per-list cheer pool by making one small
// Ollama call per kind (serially, so the model stays loaded). This is much
// more reliable than one big structured-output call, which mistral:7b tends
// to choke on with format=json.
func (o *Ollama) GenerateListCheers(ctx context.Context, listName string, items []string) (map[string][]string, error) {
	if o == nil {
		return nil, errors.New("ollama not configured")
	}
	start := time.Now()
	defer func() {
		log.Printf("ollama list-cheers %q (%d items) took %s", listName, len(items), time.Since(start))
	}()
	itemBlob := strings.Join(items, ", ")
	if itemBlob == "" {
		itemBlob = "(no items yet)"
	}
	kinds := []CheerKind{
		CheerFirst, CheerMilestone25, CheerMilestone50,
		CheerMilestone75, CheerComplete, CheerMissing,
	}
	out := map[string][]string{}
	for _, k := range kinds {
		phrases, err := o.generateListCheerKind(ctx, listName, itemBlob, k, 5)
		if err != nil {
			// partial success is still useful — return what we've got
			if len(out) > 0 {
				log.Printf("ollama list-cheers %q kind %s failed: %v (partial)", listName, k, err)
				return out, nil
			}
			return nil, err
		}
		if len(phrases) > 0 {
			out[string(k)] = phrases
		}
	}
	return out, nil
}

// generateListCheerKind generates N phrases for one kind, given list context.
func (o *Ollama) generateListCheerKind(ctx context.Context, listName, itemBlob string, kind CheerKind, n int) ([]string, error) {
	ctxDesc := KindPromptContext[kind]
	if ctxDesc == "" {
		ctxDesc = "is shopping for groceries"
	}
	prompt := fmt.Sprintf(listKindCheerTemplate, listName, itemBlob, n, ctxDesc)
	body, _ := json.Marshal(ollamaGenReq{
		Model:     o.Model,
		Prompt:    prompt,
		Format:    "json",
		Stream:    false,
		KeepAlive: "30m",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", o.URL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	var r ollamaGenResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	var parsed struct {
		Cheers []string `json:"cheers"`
	}
	if err := json.Unmarshal([]byte(r.Response), &parsed); err != nil {
		return nil, nil // bad JSON — return empty, not fatal
	}
	out := make([]string, 0, len(parsed.Cheers))
	for _, s := range parsed.Cheers {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"'`)
		if s == "" || len(s) > 80 {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// CheerStream streams a single cheer from the LLM token-by-token, calling
// onChunk for every chunk (partial text). The final call has done=true and an
// empty chunk. Respects ctx cancellation (close the pipe on client disconnect).
func (o *Ollama) CheerStream(ctx context.Context, kind CheerKind, onChunk func(chunk string, done bool) error) error {
	if o == nil {
		return errors.New("ollama not configured")
	}
	ctxDesc := KindPromptContext[kind]
	if ctxDesc == "" {
		ctxDesc = "is grocery shopping"
	}
	start := time.Now()
	defer func() {
		log.Printf("ollama cheer-stream %s took %s", kind, time.Since(start))
	}()
	prompt := fmt.Sprintf(cheerSingleTemplate, ctxDesc)
	body, _ := json.Marshal(ollamaGenReq{
		Model:     o.Model,
		Prompt:    prompt,
		Stream:    true,
		KeepAlive: "30m",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", o.URL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Streaming: override the client's request timeout by using a per-request
	// context deadline instead.
	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var r ollamaGenResp
		if err := dec.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if err := onChunk(r.Response, r.Done); err != nil {
			return err
		}
		if r.Done {
			return nil
		}
	}
	return nil
}

// GenerateCheers asks the LLM for N short cheer strings for a kind.
func (o *Ollama) GenerateCheers(ctx context.Context, kind CheerKind, n int) ([]string, error) {
	if o == nil {
		return nil, errors.New("ollama not configured")
	}
	start := time.Now()
	defer func() {
		log.Printf("ollama cheers %s took %s", kind, time.Since(start))
	}()
	ctxDesc := KindPromptContext[kind]
	if ctxDesc == "" {
		ctxDesc = "is shopping for groceries"
	}
	prompt := fmt.Sprintf(cheerSystemTemplate, n, ctxDesc)
	resp, err := o.generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Cheers []string `json:"cheers"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return nil, nil
	}
	out := make([]string, 0, len(parsed.Cheers))
	for _, s := range parsed.Cheers {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"'`)
		if s == "" || len(s) > 80 {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (o *Ollama) generate(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(ollamaGenReq{
		Model:     o.Model,
		Prompt:    prompt,
		Format:    "json",
		Stream:    false,
		KeepAlive: "30m",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", o.URL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	var r ollamaGenResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.Response, nil
}

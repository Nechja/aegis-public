package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nechja/Aegis/apps/glist/internal/ai"
	"github.com/Nechja/Aegis/apps/glist/internal/list"
	"github.com/Nechja/Aegis/apps/glist/internal/store"
)

//go:embed all:frontend/build
var frontendFS embed.FS

type Server struct {
	st     *store.Store
	ollama *ai.Ollama
	cheer  *ai.Cheer
	hub    *Hub
	mux    *http.ServeMux
}

func New(st *store.Store, ol *ai.Ollama, cheer *ai.Cheer) http.Handler {
	s := &Server{st: st, ollama: ol, cheer: cheer, hub: NewHub(), mux: http.NewServeMux()}
	s.routes()
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.healthz)

	s.mux.HandleFunc("GET /api/lists", s.listLists)
	s.mux.HandleFunc("POST /api/lists", s.createList)
	s.mux.HandleFunc("GET /api/lists/{id}", s.getList)
	s.mux.HandleFunc("PUT /api/lists/{id}", s.renameList)
	s.mux.HandleFunc("DELETE /api/lists/{id}", s.deleteList)
	s.mux.HandleFunc("POST /api/lists/{id}/reset", s.resetList)
	s.mux.HandleFunc("POST /api/lists/{id}/duplicate", s.duplicateList)
	s.mux.HandleFunc("POST /api/lists/{id}/status", s.setListStatus)
	s.mux.HandleFunc("PUT /api/lists/{id}/tags", s.setListTags)
	s.mux.HandleFunc("GET /api/lists/{id}/events", s.listEvents)

	s.mux.HandleFunc("POST /api/items", s.createItem)
	s.mux.HandleFunc("PATCH /api/items/{id}", s.patchItem)
	s.mux.HandleFunc("DELETE /api/items/{id}", s.deleteItem)

	s.mux.HandleFunc("POST /api/import", s.importList)
	s.mux.HandleFunc("POST /api/ai/categorize", s.aiCategorize)
	s.mux.HandleFunc("POST /api/ai/categorize-batch", s.aiCategorizeBatch)
	s.mux.HandleFunc("POST /api/ai/replace", s.aiReplace)
	s.mux.HandleFunc("GET /api/ai/cheer", s.aiCheer)
	s.mux.HandleFunc("GET /api/ai/cheer-stream", s.aiCheerStream)
	s.mux.HandleFunc("POST /api/lists/{id}/cheers/generate", s.generateListCheers)

	sub, err := fs.Sub(frontendFS, "frontend/build")
	if err == nil {
		s.mux.Handle("/", spaHandler(sub))
	}
}

// spaHandler serves static files when they exist and falls back to index.html
// for any other GET so client-side routes like /l/42 resolve to the SPA.
func spaHandler(fsys fs.FS) http.Handler {
	file := http.FileServer(http.FS(fsys))
	index, ierr := fs.ReadFile(fsys, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" {
			// bare / and /index.html: never cache — must pick up new builds
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			file.ServeHTTP(w, r)
			return
		}
		if f, err := fsys.Open(p); err == nil {
			_ = f.Close()
			// hashed assets can cache aggressively
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			file.ServeHTTP(w, r)
			return
		}
		// no file matched — serve index.html so the SPA can route
		if ierr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = io.Copy(w, strings.NewReader(string(index)))
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.st.Ping(ctx); err != nil {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// ---- lists ----

func (s *Server) listLists(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "archived" && status != "template" {
		http.Error(w, "bad status", http.StatusBadRequest)
		return
	}
	out, err := s.st.ListLists(r.Context(), status)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Items []struct {
			Text     string  `json:"text"`
			Category string  `json:"category"`
			Count    float64 `json:"count"`
			Unit     string  `json:"unit"`
		} `json:"items"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name == "" {
		in.Name = "new list"
	}
	l, err := s.st.CreateList(r.Context(), in.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(in.Items) > 0 {
		conv := make([]store.Item, len(in.Items))
		for i, it := range in.Items {
			cat := it.Category
			if cat == "" || !list.IsValidCategory(cat) {
				if c, ok := list.Rule(it.Text); ok {
					cat = c
				} else {
					cat = "other"
				}
			}
			count := it.Count
			if count == 0 {
				count = 1
			}
			conv[i] = store.Item{
				Text:     it.Text,
				Category: cat,
				Count:    count,
				Unit:     it.Unit,
			}
		}
		items, err := s.st.BulkCreateItems(r.Context(), l.ID, conv)
		if err != nil {
			writeErr(w, err)
			return
		}
		l.Items = items
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) getList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	l, err := s.st.GetList(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) renameList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	l, err := s.st.RenameList(r.Context(), id, in.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(id)
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteList(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := s.st.ResetList(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) duplicateList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	_ = decodeOptional(r, &in)
	if in.Name == "" {
		in.Name = "copy"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	l, err := s.st.DuplicateList(r.Context(), id, in.Name, in.Status)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) setListStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	l, err := s.st.SetListStatus(r.Context(), id, in.Status)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(id)
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) setListTags(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Tags []string `json:"tags"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	// dedupe + trim + cap
	seen := map[string]struct{}{}
	out := []string{}
	for _, t := range in.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || len(t) > 32 {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= 8 {
			break
		}
	}
	l, err := s.st.SetListTags(r.Context(), id, out)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(id)
	writeJSON(w, http.StatusOK, l)
}

// ---- items ----

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ListID   int64   `json:"list_id"`
		Text     string  `json:"text"`
		Category string  `json:"category"`
		Count    float64 `json:"count"`
		Unit     string  `json:"unit"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	text := in.Text
	count := in.Count
	unit := in.Unit
	// parse quantity out of text when caller didn't specify a count explicitly
	if count == 0 && unit == "" {
		c, u, t := list.ParseQuantity(text)
		count, unit, text = c, u, t
	}
	if count == 0 {
		count = 1
	}
	if text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	cat := in.Category
	needsBackground := false
	if cat == "" || !list.IsValidCategory(cat) {
		if c, ok := list.Rule(text); ok {
			cat = c
		} else {
			cat = "other"
			needsBackground = s.ollama != nil
		}
	}
	it, err := s.st.CreateItem(r.Context(), in.ListID, store.ItemInput{
		Text: text, Category: cat, Count: count, Unit: unit,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(it.ListID)
	if needsBackground {
		s.queueCategorize(it.ID, it.Text)
	}
	writeJSON(w, http.StatusCreated, it)
}

func (s *Server) queueCategorize(itemID int64, text string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		c, err := s.ollama.Categorize(ctx, text)
		if err != nil || c == "" || c == "other" {
			return
		}
		updated, err := s.st.UpdateItem(ctx, itemID, store.ItemPatch{Category: &c})
		if err == nil {
			s.hub.Publish(updated.ListID)
		}
	}()
}

func (s *Server) patchItem(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Text     *string  `json:"text"`
		Category *string  `json:"category"`
		Count    *float64 `json:"count"`
		Unit     *string  `json:"unit"`
		Bought   *bool    `json:"bought"`
		Missing  *bool    `json:"missing"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Category != nil && *in.Category != "" && !list.IsValidCategory(*in.Category) {
		http.Error(w, "bad category", http.StatusBadRequest)
		return
	}
	it, err := s.st.UpdateItem(r.Context(), id, store.ItemPatch{
		Text: in.Text, Category: in.Category, Count: in.Count, Unit: in.Unit,
		Bought: in.Bought, Missing: in.Missing,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(it.ListID)
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	listID, err := s.st.DeleteItem(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(listID)
	w.WriteHeader(http.StatusNoContent)
}

// ---- import / ai ----

func (s *Server) importList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Markdown string `json:"markdown"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name == "" {
		in.Name = "imported list"
	}
	parsed := list.ParseMarkdown(in.Markdown)
	items := make([]store.Item, 0, len(parsed))
	var unknownIdx []int
	var unknownText []string
	for i, p := range parsed {
		cat := p.Category
		if cat == "" || !list.IsValidCategory(cat) {
			if c, ok := list.Rule(p.Text); ok {
				cat = c
			} else {
				unknownIdx = append(unknownIdx, i)
				unknownText = append(unknownText, p.Text)
				cat = "other"
			}
		}
		count := p.Count
		if count == 0 {
			count = 1
		}
		items = append(items, store.Item{
			Text: p.Text, Category: cat, Count: count, Unit: p.Unit,
		})
	}
	if s.ollama != nil && len(unknownText) > 0 {
		aiCtx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		resolved := s.ollama.CategorizeBatch(aiCtx, unknownText)
		for j, idx := range unknownIdx {
			if resolved[j] != "" {
				items[idx].Category = resolved[j]
			}
		}
	}

	l, err := s.st.CreateList(r.Context(), in.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	stored, err := s.st.BulkCreateItems(r.Context(), l.ID, items)
	if err != nil {
		writeErr(w, err)
		return
	}
	l.Items = stored
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) aiCategorize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if c, ok := list.Rule(in.Text); ok {
		writeJSON(w, http.StatusOK, map[string]any{"category": c, "source": "rule"})
		return
	}
	if s.ollama == nil {
		writeJSON(w, http.StatusOK, map[string]any{"category": "other", "source": "none"})
		return
	}
	aiCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	c, err := s.ollama.Categorize(aiCtx, in.Text)
	if err != nil || c == "" {
		writeJSON(w, http.StatusOK, map[string]any{"category": "other", "source": "fallback"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"category": c, "source": "ollama"})
}

func (s *Server) aiCategorizeBatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items []struct {
			ID   int64  `json:"id"`
			Text string `json:"text"`
		} `json:"items"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	type out struct {
		ID       int64  `json:"id"`
		Category string `json:"category"`
		Source   string `json:"source"`
	}
	results := make([]out, len(in.Items))
	var unknownIdx []int
	var unknownText []string
	for i, it := range in.Items {
		if c, ok := list.Rule(it.Text); ok {
			results[i] = out{ID: it.ID, Category: c, Source: "rule"}
			continue
		}
		unknownIdx = append(unknownIdx, i)
		unknownText = append(unknownText, it.Text)
		results[i] = out{ID: it.ID, Category: "other", Source: "none"}
	}
	if s.ollama != nil && len(unknownText) > 0 {
		aiCtx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		resolved := s.ollama.CategorizeBatch(aiCtx, unknownText)
		for j, idx := range unknownIdx {
			if resolved[j] != "" && resolved[j] != "other" {
				results[idx].Category = resolved[j]
				results[idx].Source = "ollama"
			} else {
				results[idx].Source = "fallback"
			}
		}
	}
	writeJSON(w, http.StatusOK, results)
}

// aiCheerStream streams a fresh cheer from the LLM as SSE. Each "data:" event
// carries {chunk, done}. Frontend appends as each arrives for a typewriter
// effect. Falls back to the pool path on error.
func (s *Server) aiCheerStream(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !ai.IsValidKind(kind) {
		http.Error(w, "bad kind", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if s.ollama == nil {
		// fall back to a pool pick — emit it as a single chunk
		text := "nice"
		if s.cheer != nil {
			text = s.cheer.Pick(ai.CheerKind(kind))
		}
		fmt.Fprintf(w, "data: %s\n\n", encodeSSE(map[string]any{"chunk": text, "done": true, "source": "pool"}))
		flusher.Flush()
		return
	}

	writeChunk := func(chunk string, done bool) error {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		default:
		}
		payload := map[string]any{"chunk": chunk, "done": done, "source": "ollama"}
		fmt.Fprintf(w, "data: %s\n\n", encodeSSE(payload))
		flusher.Flush()
		return nil
	}

	// give streaming up to 90s total; Ollama cold starts can take that long
	streamCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	if err := s.ollama.CheerStream(streamCtx, ai.CheerKind(kind), writeChunk); err != nil {
		// failure — send a pool-pick as final fallback
		if s.cheer != nil {
			_ = writeChunk(s.cheer.Pick(ai.CheerKind(kind)), true)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", encodeSSE(map[string]any{"chunk": "nice", "done": true, "source": "fallback"}))
		flusher.Flush()
	}
}

func encodeSSE(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// listEvents streams change notifications for one list as SSE.
// Any mutation that touches the list (item add/update/delete, rename, tag,
// reset, status, cheer pool) causes a "update" event to be emitted. The
// client is expected to refetch the list state on each event.
func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	flusher, fok := w.(http.Flusher)
	if !fok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := s.hub.Subscribe(id)
	defer unsub()

	// initial hello so the client knows the stream is live
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprintf(w, "event: update\ndata: {}\n\n")
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": hb\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) aiCheer(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !ai.IsValidKind(kind) {
		http.Error(w, "bad kind", http.StatusBadRequest)
		return
	}

	// list-specific pool takes precedence when ?list=N is passed
	if lid := r.URL.Query().Get("list"); lid != "" {
		if id, err := strconv.ParseInt(lid, 10, 64); err == nil {
			if text, ok := s.pickFromListPool(r.Context(), id, kind); ok {
				writeJSON(w, http.StatusOK, map[string]any{"text": text, "kind": kind, "source": "list"})
				return
			}
		}
	}

	if s.cheer == nil {
		writeJSON(w, http.StatusOK, map[string]any{"text": "nice", "source": "default"})
		return
	}
	text := s.cheer.Pick(ai.CheerKind(kind))
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "kind": kind, "source": "pool"})
}

// pickFromListPool returns a random cheer from a list's stored pool, if any.
func (s *Server) pickFromListPool(ctx context.Context, listID int64, kind string) (string, bool) {
	got, err := s.st.GetListCheers(ctx, listID)
	if err != nil {
		return "", false
	}
	p := got[kind]
	if len(p) == 0 {
		return "", false
	}
	return p[int(time.Now().UnixNano())%len(p)], true
}

// generateListCheers runs an Ollama call against the list's items to produce
// a tailored cheer pool, then saves it.
func (s *Server) generateListCheers(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if s.ollama == nil {
		http.Error(w, "ai not configured", http.StatusServiceUnavailable)
		return
	}
	l, err := s.st.GetList(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	items := make([]string, 0, len(l.Items))
	for _, it := range l.Items {
		if it.Text != "" {
			items = append(items, it.Text)
		}
	}
	// 6 sequential Ollama calls × up to ~90s each worst case
	aiCtx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	pool, err := s.ollama.GenerateListCheers(aiCtx, l.Name, items)
	if err != nil {
		http.Error(w, "generation failed", http.StatusBadGateway)
		return
	}
	if len(pool) == 0 {
		http.Error(w, "empty result", http.StatusBadGateway)
		return
	}
	if err := s.st.SetListCheers(r.Context(), id, pool); err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(id)
	writeJSON(w, http.StatusOK, map[string]any{"cheers": pool})
}

func (s *Server) aiReplace(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if s.ollama == nil {
		writeJSON(w, http.StatusOK, map[string]any{"suggestions": []any{}})
		return
	}
	aiCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	ss, err := s.ollama.Replace(aiCtx, in.Text)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"suggestions": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": ss})
}

// ---- helpers ----

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return false
	}
	return true
}

func decodeOptional(r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		return false
	}
	return json.NewDecoder(r.Body).Decode(dst) == nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/Nechja/Aegis/apps/jot/internal/store"
)

//go:embed all:frontend/build
var frontendFS embed.FS

type Server struct {
	st  *store.Store
	mux *http.ServeMux
}

func New(st *store.Store) http.Handler {
	s := &Server{st: st, mux: http.NewServeMux()}
	s.routes()
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.healthz)

	s.mux.HandleFunc("GET /api/notes", s.listNotes)
	s.mux.HandleFunc("POST /api/notes", s.createNote)
	s.mux.HandleFunc("PUT /api/notes/{id}", s.updateNote)
	s.mux.HandleFunc("DELETE /api/notes/{id}", s.deleteNote)

	s.mux.HandleFunc("GET /api/todo-lists", s.listTodoLists)
	s.mux.HandleFunc("POST /api/todo-lists", s.createTodoList)
	s.mux.HandleFunc("PATCH /api/todo-lists/{id}", s.patchTodoList)
	s.mux.HandleFunc("DELETE /api/todo-lists/{id}", s.deleteTodoList)

	s.mux.HandleFunc("GET /api/todos", s.listTodos)
	s.mux.HandleFunc("POST /api/todos", s.createTodo)
	s.mux.HandleFunc("PATCH /api/todos/{id}", s.patchTodo)
	s.mux.HandleFunc("DELETE /api/todos/{id}", s.deleteTodo)

	sub, err := fs.Sub(frontendFS, "frontend/build")
	if err == nil {
		s.mux.Handle("/", http.FileServer(http.FS(sub)))
	}
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

// ---- notes ----

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.st.ListNotes(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	var in struct{ Title, Body string }
	if !readJSON(w, r, &in) {
		return
	}
	n, err := s.st.CreateNote(r.Context(), in.Title, in.Body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct{ Title, Body string }
	if !readJSON(w, r, &in) {
		return
	}
	n, err := s.st.UpdateNote(r.Context(), id, in.Title, in.Body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteNote(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- todo lists ----

func (s *Server) listTodoLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.st.ListTodoLists(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (s *Server) createTodoList(w http.ResponseWriter, r *http.Request) {
	var in struct{ Title string `json:"title"` }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	tl, err := s.st.CreateTodoList(r.Context(), in.Title)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tl)
}

func (s *Server) patchTodoList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct{ Title string `json:"title"` }
	if !readJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	tl, err := s.st.UpdateTodoList(r.Context(), id, in.Title)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

func (s *Server) deleteTodoList(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteTodoList(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- todos ----

func (s *Server) listTodos(w http.ResponseWriter, r *http.Request) {
	var todos []store.Todo
	var err error
	if q := r.URL.Query().Get("list_id"); q != "" {
		id, parseErr := strconv.ParseInt(q, 10, 64)
		if parseErr != nil {
			http.Error(w, "bad list_id", http.StatusBadRequest)
			return
		}
		todos, err = s.st.ListTodosByList(r.Context(), id)
	} else {
		todos, err = s.st.ListTodos(r.Context())
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, todos)
}

func (s *Server) createTodo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text   string `json:"text"`
		ListID *int64 `json:"list_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	t, err := s.st.CreateTodo(r.Context(), in.ListID, in.Text)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) patchTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Text   *string `json:"text"`
		Done   *bool   `json:"done"`
		ListID *int64  `json:"list_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.st.UpdateTodo(r.Context(), id, store.TodoPatch{Text: in.Text, Done: in.Done, ListID: in.ListID})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := s.st.DeleteTodo(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Note struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TodoList struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Todo struct {
	ID        int64     `json:"id"`
	ListID    *int64    `json:"list_id"`
	Text      string    `json:"text"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS notes (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS todo_lists (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS todos (
    id BIGSERIAL PRIMARY KEY,
    list_id BIGINT REFERENCES todo_lists(id) ON DELETE SET NULL,
    text TEXT NOT NULL,
    done BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE IF EXISTS todos 
ADD COLUMN IF NOT EXISTS list_id BIGINT REFERENCES todo_lists(id) ON DELETE SET NULL;

DO $$
DECLARE
    inbox_id BIGINT;
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'todos'
    ) AND EXISTS (SELECT 1 FROM todos WHERE list_id IS NULL) THEN
        INSERT INTO todo_lists (title) VALUES ('Inbox') RETURNING id INTO inbox_id;
        UPDATE todos SET list_id = inbox_id WHERE list_id IS NULL;
    END IF;
END $$;
`

func (s *Store) Migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7274737374)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ---- notes ----

func (s *Store) ListNotes(ctx context.Context) ([]Note, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, body, created_at, updated_at FROM notes ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CreateNote(ctx context.Context, title, body string) (Note, error) {
	var n Note
	err := s.pool.QueryRow(ctx,
		`INSERT INTO notes (title, body) VALUES ($1, $2) RETURNING id, title, body, created_at, updated_at`,
		title, body,
	).Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

func (s *Store) UpdateNote(ctx context.Context, id int64, title, body string) (Note, error) {
	var n Note
	err := s.pool.QueryRow(ctx,
		`UPDATE notes SET title = $2, body = $3, updated_at = now() WHERE id = $1
		 RETURNING id, title, body, created_at, updated_at`,
		id, title, body,
	).Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

func (s *Store) DeleteNote(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- todo lists ----

func (s *Store) ListTodoLists(ctx context.Context) ([]TodoList, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, created_at, updated_at FROM todo_lists ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TodoList{}
	for rows.Next() {
		var tl TodoList
		if err := rows.Scan(&tl.ID, &tl.Title, &tl.CreatedAt, &tl.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, tl)
	}
	return out, rows.Err()
}

func (s *Store) CreateTodoList(ctx context.Context, title string) (TodoList, error) {
	var tl TodoList
	err := s.pool.QueryRow(ctx,
		`INSERT INTO todo_lists (title) VALUES ($1) RETURNING id, title, created_at, updated_at`,
		title,
	).Scan(&tl.ID, &tl.Title, &tl.CreatedAt, &tl.UpdatedAt)
	return tl, err
}

func (s *Store) UpdateTodoList(ctx context.Context, id int64, title string) (TodoList, error) {
	var tl TodoList
	err := s.pool.QueryRow(ctx,
		`UPDATE todo_lists SET title = $2, updated_at = now() WHERE id = $1 RETURNING id, title, created_at, updated_at`,
		id, title,
	).Scan(&tl.ID, &tl.Title, &tl.CreatedAt, &tl.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tl, ErrNotFound
	}
	return tl, err
}

func (s *Store) DeleteTodoList(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM todo_lists WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- todos ----

func (s *Store) ListTodos(ctx context.Context) ([]Todo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, list_id, text, done, created_at, updated_at FROM todos ORDER BY done ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Todo{}
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.ListID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListTodosByList(ctx context.Context, listID int64) ([]Todo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, list_id, text, done, created_at, updated_at FROM todos WHERE list_id = $1 ORDER BY done ASC, created_at DESC`,
		listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Todo{}
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.ListID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CreateTodo(ctx context.Context, listID *int64, text string) (Todo, error) {
	var t Todo
	err := s.pool.QueryRow(ctx,
		`INSERT INTO todos (list_id, text) VALUES ($1, $2) RETURNING id, list_id, text, done, created_at, updated_at`,
		listID, text,
	).Scan(&t.ID, &t.ListID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

type TodoPatch struct {
	Text   *string
	Done   *bool
	ListID *int64
}

func (s *Store) UpdateTodo(ctx context.Context, id int64, p TodoPatch) (Todo, error) {
	var t Todo
	err := s.pool.QueryRow(ctx,
		`UPDATE todos
		 SET text = COALESCE($2, text),
		     done = COALESCE($3, done),
		     list_id = COALESCE($4, list_id),
		     updated_at = now()
		 WHERE id = $1
		 RETURNING id, list_id, text, done, created_at, updated_at`,
		id, p.Text, p.Done, p.ListID,
	).Scan(&t.ID, &t.ListID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (s *Store) DeleteTodo(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

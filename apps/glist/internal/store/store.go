package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type List struct {
	ID          int64               `json:"id"`
	Name        string              `json:"name"`
	Status      string              `json:"status"` // active | archived | template
	Tags        []string            `json:"tags"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
	Items       []Item              `json:"items,omitempty"`
	ItemCount   int                 `json:"item_count,omitempty"`
	Cheers      map[string][]string `json:"cheers,omitempty"`
}

type Item struct {
	ID        int64      `json:"id"`
	ListID    int64      `json:"list_id"`
	Text      string     `json:"text"`
	Category  string     `json:"category"`
	Count     float64    `json:"count"`
	Unit      string     `json:"unit"`
	Bought    bool       `json:"bought"`
	Missing   bool       `json:"missing"`
	SortIdx   int        `json:"sort_idx"`
	CreatedAt time.Time  `json:"created_at"`
	BoughtAt  *time.Time `json:"bought_at,omitempty"`
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
CREATE TABLE IF NOT EXISTS lists (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS items (
    id BIGSERIAL PRIMARY KEY,
    list_id BIGINT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'other',
    count DOUBLE PRECISION NOT NULL DEFAULT 1,
    unit TEXT NOT NULL DEFAULT '',
    bought BOOLEAN NOT NULL DEFAULT false,
    missing BOOLEAN NOT NULL DEFAULT false,
    sort_idx INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS items_list_idx ON items(list_id);
ALTER TABLE lists ADD COLUMN IF NOT EXISTS cheers JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE lists ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE items ADD COLUMN IF NOT EXISTS bought_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS lists_status_idx ON lists(status);
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
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(0x676c6973744d)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ---- lists ----

// ListLists returns lists filtered by status (active | archived | template).
// Empty string returns active.
func (s *Store) ListLists(ctx context.Context, status string) ([]List, error) {
	if status == "" {
		status = "active"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.name, l.status, l.tags, l.created_at, l.updated_at, l.completed_at,
		       COALESCE(c.n, 0) AS item_count
		FROM lists l
		LEFT JOIN (SELECT list_id, COUNT(*) AS n FROM items GROUP BY list_id) c
		  ON c.list_id = l.id
		WHERE l.status = $1
		ORDER BY l.updated_at DESC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []List{}
	for rows.Next() {
		var l List
		var tagsRaw []byte
		if err := rows.Scan(&l.ID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt, &l.ItemCount); err != nil {
			return nil, err
		}
		l.Tags = decodeTags(tagsRaw)
		out = append(out, l)
	}
	return out, rows.Err()
}

func decodeTags(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return []string{}
	}
	return out
}

func (s *Store) CreateList(ctx context.Context, name string) (List, error) {
	var l List
	var tagsRaw []byte
	err := s.pool.QueryRow(ctx,
		`INSERT INTO lists (name) VALUES ($1)
		 RETURNING id, name, status, tags, created_at, updated_at, completed_at`,
		name,
	).Scan(&l.ID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt)
	l.Tags = decodeTags(tagsRaw)
	return l, err
}

func (s *Store) RenameList(ctx context.Context, id int64, name string) (List, error) {
	var l List
	var tagsRaw []byte
	err := s.pool.QueryRow(ctx,
		`UPDATE lists SET name = $2, updated_at = now() WHERE id = $1
		 RETURNING id, name, status, tags, created_at, updated_at, completed_at`,
		id, name,
	).Scan(&l.ID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	l.Tags = decodeTags(tagsRaw)
	return l, err
}

// SetListStatus transitions a list between active/archived/template.
// Also stamps completed_at on archive if it's still null and the list is fully bought.
func (s *Store) SetListStatus(ctx context.Context, id int64, status string) (List, error) {
	if status != "active" && status != "archived" && status != "template" {
		return List{}, errors.New("bad status")
	}
	var l List
	var tagsRaw []byte
	err := s.pool.QueryRow(ctx,
		`UPDATE lists SET status = $2, updated_at = now(),
		     completed_at = CASE
		       WHEN $2 = 'archived' AND completed_at IS NULL THEN now()
		       ELSE completed_at
		     END
		 WHERE id = $1
		 RETURNING id, name, status, tags, created_at, updated_at, completed_at`,
		id, status,
	).Scan(&l.ID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	l.Tags = decodeTags(tagsRaw)
	return l, err
}

// SetListTags replaces the tag array.
func (s *Store) SetListTags(ctx context.Context, id int64, tags []string) (List, error) {
	raw, _ := json.Marshal(tags)
	var l List
	var tagsRaw []byte
	err := s.pool.QueryRow(ctx,
		`UPDATE lists SET tags = $2::jsonb, updated_at = now() WHERE id = $1
		 RETURNING id, name, status, tags, created_at, updated_at, completed_at`,
		id, raw,
	).Scan(&l.ID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	l.Tags = decodeTags(tagsRaw)
	return l, err
}

// MarkCompleted stamps completed_at = now() if it's null. Called when the
// client notices 100% of items are bought.
func (s *Store) MarkCompleted(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE lists SET completed_at = COALESCE(completed_at, now()), updated_at = now()
		 WHERE id = $1`, id)
	return err
}

func (s *Store) DeleteList(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM lists WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetList(ctx context.Context, id int64) (List, error) {
	var l List
	var cheersRaw, tagsRaw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, status, tags, created_at, updated_at, completed_at, cheers
		 FROM lists WHERE id = $1`,
		id,
	).Scan(&l.ID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt, &cheersRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	l.Tags = decodeTags(tagsRaw)
	if len(cheersRaw) > 0 {
		_ = json.Unmarshal(cheersRaw, &l.Cheers)
	}
	items, err := s.ListItems(ctx, id)
	if err != nil {
		return l, err
	}
	l.Items = items
	return l, nil
}

// SetListCheers replaces the stored cheer pool for a list.
func (s *Store) SetListCheers(ctx context.Context, id int64, cheers map[string][]string) error {
	raw, err := json.Marshal(cheers)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE lists SET cheers = $2::jsonb, updated_at = now() WHERE id = $1`,
		id, raw,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetListCheers returns just the stored cheer pool for a list (skips items).
func (s *Store) GetListCheers(ctx context.Context, id int64) (map[string][]string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT cheers FROM lists WHERE id = $1`, id,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return map[string][]string{}, nil
	}
	out := map[string][]string{}
	_ = json.Unmarshal(raw, &out)
	return out, nil
}

func (s *Store) ResetList(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE items SET bought = false, missing = false, bought_at = NULL WHERE list_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE lists SET completed_at = NULL, updated_at = now() WHERE id = $1`, id)
	return err
}

// DuplicateList creates a new list copying the source's items (unchecked) and
// tags. `status` controls what state the new list lands in ("active" to clone
// normally, "template" to save as a reusable template).
func (s *Store) DuplicateList(ctx context.Context, id int64, newName, status string) (List, error) {
	if status == "" {
		status = "active"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return List{}, err
	}
	defer tx.Rollback(ctx)
	var newID int64
	var l List
	var tagsRaw []byte
	err = tx.QueryRow(ctx,
		`INSERT INTO lists (name, status, tags)
		 SELECT $1, $3, tags FROM lists WHERE id = $2
		 RETURNING id, name, status, tags, created_at, updated_at, completed_at`,
		newName, id, status,
	).Scan(&newID, &l.Name, &l.Status, &tagsRaw, &l.CreatedAt, &l.UpdatedAt, &l.CompletedAt)
	if err != nil {
		return List{}, err
	}
	l.ID = newID
	l.Tags = decodeTags(tagsRaw)
	_, err = tx.Exec(ctx,
		`INSERT INTO items (list_id, text, category, count, unit, bought, missing, sort_idx)
		 SELECT $2, text, category, count, unit, false, false, sort_idx
		 FROM items WHERE list_id = $1`,
		id, newID,
	)
	if err != nil {
		return List{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return List{}, err
	}
	return l, nil
}

// ---- items ----

func (s *Store) ListItems(ctx context.Context, listID int64) ([]Item, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, list_id, text, category, count, unit, bought, missing, sort_idx, created_at, bought_at
		 FROM items WHERE list_id = $1
		 ORDER BY sort_idx ASC, id ASC`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.ListID, &it.Text, &it.Category, &it.Count, &it.Unit, &it.Bought, &it.Missing, &it.SortIdx, &it.CreatedAt, &it.BoughtAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

type ItemInput struct {
	Text     string
	Category string
	Count    float64
	Unit     string
}

func (s *Store) CreateItem(ctx context.Context, listID int64, in ItemInput) (Item, error) {
	if in.Count == 0 {
		in.Count = 1
	}
	var it Item
	err := s.pool.QueryRow(ctx,
		`INSERT INTO items (list_id, text, category, count, unit)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, list_id, text, category, count, unit, bought, missing, sort_idx, created_at, bought_at`,
		listID, in.Text, in.Category, in.Count, in.Unit,
	).Scan(&it.ID, &it.ListID, &it.Text, &it.Category, &it.Count, &it.Unit, &it.Bought, &it.Missing, &it.SortIdx, &it.CreatedAt, &it.BoughtAt)
	if err == nil {
		_, _ = s.pool.Exec(ctx, `UPDATE lists SET updated_at = now() WHERE id = $1`, listID)
	}
	return it, err
}

type ItemPatch struct {
	Text     *string
	Category *string
	Count    *float64
	Unit     *string
	Bought   *bool
	Missing  *bool
}

// maybeStampCompleted sets lists.completed_at to now() if all items are bought
// and it's not already stamped. Called after item updates.
func (s *Store) maybeStampCompleted(ctx context.Context, listID int64) {
	_, _ = s.pool.Exec(ctx, `
		UPDATE lists SET completed_at = now(), updated_at = now()
		WHERE id = $1
		  AND completed_at IS NULL
		  AND EXISTS(SELECT 1 FROM items WHERE list_id = $1)
		  AND NOT EXISTS(SELECT 1 FROM items WHERE list_id = $1 AND bought = false)`, listID)
}

func (s *Store) UpdateItem(ctx context.Context, id int64, p ItemPatch) (Item, error) {
	var it Item
	// bought_at stamp: set to now when bought flips on, null when it flips off
	err := s.pool.QueryRow(ctx,
		`UPDATE items
		 SET text     = COALESCE($2, text),
		     category = COALESCE($3, category),
		     count    = COALESCE($4, count),
		     unit     = COALESCE($5, unit),
		     bought   = COALESCE($6, bought),
		     missing  = COALESCE($7, missing),
		     bought_at = CASE
		       WHEN $6::boolean IS TRUE THEN COALESCE(bought_at, now())
		       WHEN $6::boolean IS FALSE THEN NULL
		       ELSE bought_at
		     END
		 WHERE id = $1
		 RETURNING id, list_id, text, category, count, unit, bought, missing, sort_idx, created_at, bought_at`,
		id, p.Text, p.Category, p.Count, p.Unit, p.Bought, p.Missing,
	).Scan(&it.ID, &it.ListID, &it.Text, &it.Category, &it.Count, &it.Unit, &it.Bought, &it.Missing, &it.SortIdx, &it.CreatedAt, &it.BoughtAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	if err == nil && p.Bought != nil && *p.Bought {
		s.maybeStampCompleted(ctx, it.ListID)
	}
	return it, err
}

// DeleteItem removes the item and returns its list_id so callers can publish
// a change notification.
func (s *Store) DeleteItem(ctx context.Context, id int64) (int64, error) {
	var listID int64
	err := s.pool.QueryRow(ctx,
		`DELETE FROM items WHERE id = $1 RETURNING list_id`, id,
	).Scan(&listID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return listID, err
}

func (s *Store) BulkCreateItems(ctx context.Context, listID int64, items []Item) ([]Item, error) {
	if len(items) == 0 {
		return nil, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out := make([]Item, 0, len(items))
	for i, in := range items {
		cat := in.Category
		if cat == "" {
			cat = "other"
		}
		count := in.Count
		if count == 0 {
			count = 1
		}
		var it Item
		err := tx.QueryRow(ctx,
			`INSERT INTO items (list_id, text, category, count, unit, sort_idx)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id, list_id, text, category, count, unit, bought, missing, sort_idx, created_at, bought_at`,
			listID, in.Text, cat, count, in.Unit, i,
		).Scan(&it.ID, &it.ListID, &it.Text, &it.Category, &it.Count, &it.Unit, &it.Bought, &it.Missing, &it.SortIdx, &it.CreatedAt, &it.BoughtAt)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if _, err := tx.Exec(ctx, `UPDATE lists SET updated_at = now() WHERE id = $1`, listID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type Series struct {
	ID         int
	Popularity float64
	Titles     map[string]string
	FetchedAt  time.Time
	LastSeenAt time.Time
}

type BootstrapStatus struct {
	ExportDate string
	Cursor     int
	Completed  bool
	UpdatedAt  time.Time
}

type RunState struct {
	LastAttemptedRunAt    time.Time
	LastSuccessfulFetchAt time.Time
	LastGeneratedAt       time.Time
	EntryCount            int
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	out := &DB{db: db}
	if err := out.init(); err != nil {
		db.Close()
		return nil, err
	}
	return out, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) init() error {
	_, err := d.db.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS bootstrap_state (
  export_date TEXT PRIMARY KEY,
  cursor_offset INTEGER NOT NULL DEFAULT 0,
  completed INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS series (
  tmdb_id INTEGER PRIMARY KEY,
  popularity REAL NOT NULL DEFAULT 0,
  fetched_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS series_titles (
  tmdb_id INTEGER NOT NULL,
  language TEXT NOT NULL,
  title TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (tmdb_id, language),
  FOREIGN KEY (tmdb_id) REFERENCES series(tmdb_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS run_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_attempted_run_at TEXT NOT NULL DEFAULT '',
  last_successful_fetch_at TEXT NOT NULL DEFAULT '',
  last_generated_at TEXT NOT NULL DEFAULT '',
  entry_count INTEGER NOT NULL DEFAULT 0
);
`)
	return err
}

func (d *DB) RunState() (RunState, error) {
	row := d.db.QueryRow(`
SELECT last_attempted_run_at, last_successful_fetch_at, last_generated_at, entry_count
FROM run_state
WHERE id = 1
`)
	var attemptedText, successfulText, generatedText string
	var state RunState
	if err := row.Scan(&attemptedText, &successfulText, &generatedText, &state.EntryCount); err != nil {
		if err == sql.ErrNoRows {
			return RunState{}, nil
		}
		return RunState{}, err
	}
	state.LastAttemptedRunAt = parseTime(attemptedText)
	state.LastSuccessfulFetchAt = parseTime(successfulText)
	state.LastGeneratedAt = parseTime(generatedText)
	return state, nil
}

func (d *DB) SaveRunState(state RunState) error {
	_, err := d.db.Exec(`
INSERT INTO run_state (id, last_attempted_run_at, last_successful_fetch_at, last_generated_at, entry_count)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  last_attempted_run_at = excluded.last_attempted_run_at,
  last_successful_fetch_at = excluded.last_successful_fetch_at,
  last_generated_at = excluded.last_generated_at,
  entry_count = excluded.entry_count
`, formatDBTime(state.LastAttemptedRunAt), formatDBTime(state.LastSuccessfulFetchAt), formatDBTime(state.LastGeneratedAt), state.EntryCount)
	return err
}

func (d *DB) Bootstrap(exportDate string) (cursor int, completed bool, err error) {
	row := d.db.QueryRow(`SELECT cursor_offset, completed FROM bootstrap_state WHERE export_date = ?`, exportDate)
	var completedInt int
	if err := row.Scan(&cursor, &completedInt); err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}
	return cursor, completedInt != 0, nil
}

func (d *DB) SetBootstrap(exportDate string, cursor int, completed bool) error {
	completedInt := 0
	if completed {
		completedInt = 1
	}
	_, err := d.db.Exec(`
INSERT INTO bootstrap_state (export_date, cursor_offset, completed, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(export_date) DO UPDATE SET
  cursor_offset = excluded.cursor_offset,
  completed = excluded.completed,
  updated_at = excluded.updated_at
`, exportDate, cursor, completedInt, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (d *DB) LatestBootstrap() (BootstrapStatus, bool, error) {
	row := d.db.QueryRow(`
SELECT export_date, cursor_offset, completed, updated_at
FROM bootstrap_state
ORDER BY updated_at DESC
LIMIT 1
`)
	var status BootstrapStatus
	var completedInt int
	var updatedAtText string
	if err := row.Scan(&status.ExportDate, &status.Cursor, &completedInt, &updatedAtText); err != nil {
		if err == sql.ErrNoRows {
			return BootstrapStatus{}, false, nil
		}
		return BootstrapStatus{}, false, err
	}
	updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtText)
	status.Completed = completedInt != 0
	status.UpdatedAt = updatedAt
	return status, true, nil
}

func (d *DB) SeriesCount() (int, error) {
	row := d.db.QueryRow(`SELECT COUNT(*) FROM series`)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *DB) UpsertSeries(series Series) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fetchedAt := series.FetchedAt.UTC().Format(time.RFC3339Nano)
	lastSeenAt := series.LastSeenAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`
INSERT INTO series (tmdb_id, popularity, fetched_at, last_seen_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(tmdb_id) DO UPDATE SET
  popularity = excluded.popularity,
  fetched_at = excluded.fetched_at,
  last_seen_at = excluded.last_seen_at
`, series.ID, series.Popularity, fetchedAt, lastSeenAt); err != nil {
		return err
	}
	for language, title := range series.Titles {
		if _, err := tx.Exec(`
INSERT INTO series_titles (tmdb_id, language, title, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(tmdb_id, language) DO UPDATE SET
  title = excluded.title,
  updated_at = excluded.updated_at
`, series.ID, language, title, fetchedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) AllSeries() ([]Series, error) {
	rows, err := d.db.Query(`
SELECT s.tmdb_id, s.popularity, s.fetched_at, s.last_seen_at, t.language, t.title
FROM series s
LEFT JOIN series_titles t ON t.tmdb_id = s.tmdb_id
ORDER BY s.tmdb_id, t.language
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int]*Series)
	var ordered []int
	for rows.Next() {
		var id int
		var popularity float64
		var fetchedAtText, lastSeenAtText string
		var language, title sql.NullString
		if err := rows.Scan(&id, &popularity, &fetchedAtText, &lastSeenAtText, &language, &title); err != nil {
			return nil, err
		}
		series := byID[id]
		if series == nil {
			fetchedAt, _ := time.Parse(time.RFC3339Nano, fetchedAtText)
			lastSeenAt, _ := time.Parse(time.RFC3339Nano, lastSeenAtText)
			series = &Series{ID: id, Popularity: popularity, Titles: map[string]string{}, FetchedAt: fetchedAt, LastSeenAt: lastSeenAt}
			byID[id] = series
			ordered = append(ordered, id)
		}
		if language.Valid && title.Valid {
			series.Titles[language.String] = title.String
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Series, 0, len(ordered))
	for _, id := range ordered {
		out = append(out, *byID[id])
	}
	return out, nil
}

func formatDBTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

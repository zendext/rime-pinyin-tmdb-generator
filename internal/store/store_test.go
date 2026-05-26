package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStorePersistsBootstrapCursorAndSeriesTitles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "series.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SetBootstrap("05_26_2026", 42, false); err != nil {
		t.Fatal(err)
	}
	cursor, completed, err := db.Bootstrap("05_26_2026")
	if err != nil {
		t.Fatal(err)
	}
	if cursor != 42 || completed {
		t.Fatalf("unexpected bootstrap state cursor=%d completed=%v", cursor, completed)
	}

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertSeries(Series{
		ID:         101,
		Popularity: 12.5,
		Titles: map[string]string{
			"zh-CN": "虚构剧集",
			"zh-TW": "虛構劇集",
		},
		FetchedAt:  now,
		LastSeenAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.AllSeries()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 series, got %#v", got)
	}
	if got[0].ID != 101 || got[0].Titles["zh-CN"] != "虚构剧集" || got[0].Titles["zh-TW"] != "虛構劇集" {
		t.Fatalf("unexpected series: %#v", got[0])
	}

	count, err := db.SeriesCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected series count 1, got %d", count)
	}

	status, ok, err := db.LatestBootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || status.ExportDate != "05_26_2026" || status.Cursor != 42 || status.Completed {
		t.Fatalf("unexpected latest bootstrap status: ok=%v status=%#v", ok, status)
	}
}

func TestSQLiteStorePersistsRunState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "series.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	want := RunState{
		LastAttemptedRunAt:    now,
		LastSuccessfulFetchAt: now.Add(-time.Hour),
		LastGeneratedAt:       now.Add(-time.Minute),
		EntryCount:            42,
	}
	if err := db.SaveRunState(want); err != nil {
		t.Fatal(err)
	}
	got, err := db.RunState()
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastAttemptedRunAt.Equal(want.LastAttemptedRunAt) ||
		!got.LastSuccessfulFetchAt.Equal(want.LastSuccessfulFetchAt) ||
		!got.LastGeneratedAt.Equal(want.LastGeneratedAt) ||
		got.EntryCount != want.EntryCount {
		t.Fatalf("run state mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

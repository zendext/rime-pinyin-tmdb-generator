package tmdb

import (
	"reflect"
	"testing"
	"time"
)

func TestExtractChineseTitlesKeepsOnlyChineseNamesAndDeduplicates(t *testing.T) {
	records := []SeriesRecord{
		{
			ID:          101,
			Name:        "Imaginary Show",
			LastUpdated: time.Unix(1710000000, 0).UTC(),
			Translations: map[string]Translation{
				"zh-CN": {Name: "虚构剧集"},
			},
			Aliases: []Alias{
				{Name: "虚构剧集", Language: "zh-CN"},
				{Name: "另一译名", Language: "zh-CN"},
				{Name: "English Alias", Language: "eng"},
			},
		},
		{
			ID:   102,
			Name: "No Chinese Here",
			Aliases: []Alias{
				{Name: "Drama", Language: "eng"},
			},
		},
		{
			ID:   103,
			Name: "二号节目",
		},
	}

	got := ExtractChineseTitles(records, []string{"zh-CN"})
	want := []Title{
		{Text: "二号节目", SourceID: 103, Weight: 90},
		{Text: "另一译名", SourceID: 101, Weight: 80},
		{Text: "虚构剧集", SourceID: 101, Weight: 100},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("titles mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestSinceFromStateReturnsZeroWhenStateIsEmpty(t *testing.T) {
	got := SinceFromState(StateSnapshot{})
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %s", got)
	}
}

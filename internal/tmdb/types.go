package tmdb

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

type SeriesRecord struct {
	ID           int
	Name         string
	Aliases      []Alias
	Translations map[string]Translation
	LastUpdated  time.Time
}

type Alias struct {
	Name     string
	Language string
}

type Translation struct {
	Name string
}

type Title struct {
	Text     string
	SourceID int
	Weight   int
}

type TitleGroup struct {
	Name      string
	Languages []string
	Titles    []Title
}

type StateSnapshot struct {
	LastSuccessfulFetchAt time.Time
}

func SinceFromState(snapshot StateSnapshot) time.Time {
	return snapshot.LastSuccessfulFetchAt
}

func ExtractChineseTitles(records []SeriesRecord, languages []string) []Title {
	return extractChineseTitles(records, languages, true, true)
}

func extractChineseTitles(records []SeriesRecord, languages []string, includeFallbackName, includeUnlabeledAliases bool) []Title {
	allowed := languageSet(languages)
	seen := make(map[string]Title)
	for _, record := range records {
		add := func(text string, weight int) {
			text = cleanTitle(text)
			if text == "" || !containsHan(text) {
				return
			}
			if existing, ok := seen[text]; ok && existing.Weight >= weight {
				return
			}
			seen[text] = Title{Text: text, SourceID: record.ID, Weight: weight}
		}

		for lang, translation := range record.Translations {
			if len(allowed) == 0 || allowed[normalLang(lang)] {
				add(translation.Name, 100)
			}
		}
		for _, alias := range record.Aliases {
			if len(allowed) == 0 || allowed[normalLang(alias.Language)] || (includeUnlabeledAliases && alias.Language == "") {
				add(alias.Name, 80)
			}
		}
		if includeFallbackName {
			add(record.Name, 90)
		}
	}

	titles := make([]Title, 0, len(seen))
	for _, title := range seen {
		titles = append(titles, title)
	}
	sort.Slice(titles, func(i, j int) bool {
		return titles[i].Text < titles[j].Text
	})
	return titles
}

func ExtractChineseTitleGroups(records []SeriesRecord, groups []TitleGroup) []TitleGroup {
	out := make([]TitleGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, TitleGroup{
			Name:      group.Name,
			Languages: append([]string(nil), group.Languages...),
			Titles:    extractChineseTitles(records, group.Languages, group.Name == "tmdb_hans", false),
		})
	}
	return out
}

func cleanTitle(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func languageSet(languages []string) map[string]bool {
	out := make(map[string]bool, len(languages))
	for _, language := range languages {
		if language = normalLang(language); language != "" {
			out[language] = true
		}
	}
	return out
}

func normalLang(language string) string {
	return strings.ToLower(strings.TrimSpace(language))
}

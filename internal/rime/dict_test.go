package rime

import (
	"strings"
	"testing"
)

type fakeEncoder map[string]string

func (f fakeEncoder) Encode(word string) (string, bool) {
	p, ok := f[word]
	return p, ok
}

func TestBuildDictionaryUsesOverridesBeforeAutomaticPinyin(t *testing.T) {
	dict, err := BuildDictionary(BuildRequest{
		Name:    "tmdb",
		Version: "test",
		Words: []Word{
			{Text: "长安剧场", Weight: 100},
			{Text: "虚构剧集", Weight: 90},
		},
		Overrides: map[string]string{
			"长安剧场": "chang an ju chang",
		},
		Encoder: fakeEncoder{
			"长安剧场": "zhang an ju chang",
			"虚构剧集": "xu gou ju ji",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	mustContain(t, dict, "name: tmdb")
	mustContain(t, dict, "长安剧场\tchang an ju chang\t100")
	mustContain(t, dict, "虚构剧集\txu gou ju ji\t90")
	if strings.Contains(dict, "zhang an ju chang") {
		t.Fatalf("automatic pinyin should not override manual correction:\n%s", dict)
	}
}

func TestBuildDictionaryKeepsBareWordWhenEncodingFails(t *testing.T) {
	dict, err := BuildDictionary(BuildRequest{
		Name:    "tmdb",
		Version: "test",
		Words:   []Word{{Text: "A计划", Weight: 80}},
		Encoder: fakeEncoder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, dict, "A计划\t80")
}

func mustContain(t *testing.T, s, needle string) {
	t.Helper()
	if !strings.Contains(s, needle) {
		t.Fatalf("expected %q in:\n%s", needle, s)
	}
}

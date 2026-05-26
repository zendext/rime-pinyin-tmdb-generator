package rime

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Encoder interface {
	Encode(word string) (string, bool)
}

type Word struct {
	Text   string
	Weight int
}

type BuildRequest struct {
	Name      string
	Version   string
	Words     []Word
	Overrides map[string]string
	Encoder   Encoder
}

func BuildDictionary(req BuildRequest) (string, error) {
	if req.Name == "" {
		return "", errors.New("dictionary name is required")
	}
	if req.Version == "" {
		return "", errors.New("dictionary version is required")
	}

	words := dedupeWords(req.Words)
	sort.Slice(words, func(i, j int) bool {
		return words[i].Text < words[j].Text
	})

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Rime dictionary\n# encoding: utf-8\n---\nname: %s\nversion: %q\nsort: by_weight\n...\n", req.Name, req.Version)
	for _, word := range words {
		text := strings.TrimSpace(word.Text)
		if text == "" {
			continue
		}
		if pinyin := strings.TrimSpace(req.Overrides[text]); pinyin != "" {
			fmt.Fprintf(&buf, "%s\t%s\t%d\n", text, pinyin, normalizedWeight(word.Weight))
			continue
		}
		if req.Encoder != nil {
			if pinyin, ok := req.Encoder.Encode(text); ok && strings.TrimSpace(pinyin) != "" {
				fmt.Fprintf(&buf, "%s\t%s\t%d\n", text, pinyin, normalizedWeight(word.Weight))
				continue
			}
		}
		fmt.Fprintf(&buf, "%s\t%d\n", text, normalizedWeight(word.Weight))
	}
	return buf.String(), nil
}

func dedupeWords(words []Word) []Word {
	seen := make(map[string]Word, len(words))
	for _, word := range words {
		word.Text = strings.TrimSpace(word.Text)
		if word.Text == "" {
			continue
		}
		if existing, ok := seen[word.Text]; ok && existing.Weight >= word.Weight {
			continue
		}
		seen[word.Text] = word
	}
	out := make([]Word, 0, len(seen))
	for _, word := range seen {
		out = append(out, word)
	}
	return out
}

func normalizedWeight(weight int) int {
	if weight <= 0 {
		return 80
	}
	return weight
}

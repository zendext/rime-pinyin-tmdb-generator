package pinyin

import (
	"strings"
	"unicode"

	gopinyin "github.com/mozillazg/go-pinyin"
)

type Encoder struct {
	args gopinyin.Args
}

func NewEncoder() Encoder {
	args := gopinyin.NewArgs()
	args.Style = gopinyin.Normal
	args.Heteronym = false
	return Encoder{args: args}
}

func (e Encoder) Encode(word string) (string, bool) {
	word = strings.TrimSpace(word)
	if word == "" {
		return "", false
	}
	parts := gopinyin.LazyPinyin(word, e.args)
	if len(parts) == 0 {
		return "", false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || containsHan(part) {
			return "", false
		}
	}
	return strings.Join(parts, " "), true
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

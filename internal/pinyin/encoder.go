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
	if parts, ok := encodeDayNightTitle(word); ok {
		return strings.Join(parts, " "), true
	}
	runes := []rune(word)
	parts := make([]string, 0, len(runes))
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case isASCIILetter(r):
			start := i
			for i < len(runes) && isASCIILetter(runes[i]) {
				i++
			}
			parts = append(parts, strings.ToLower(string(runes[start:i])))
		case isASCIIDigit(r):
			start := i
			for i < len(runes) && isASCIIDigit(runes[i]) {
				i++
			}
			parts = append(parts, numberPinyin(string(runes[start:i]), false)...)
		case unicode.Is(unicode.Han, r):
			pinyin := gopinyin.LazyPinyin(string(r), e.args)
			if len(pinyin) == 0 {
				return "", false
			}
			parts = append(parts, pinyin...)
			i++
		default:
			i++
		}
	}
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

func encodeDayNightTitle(word string) ([]string, bool) {
	runes := []rune(word)
	firstEnd := 0
	for firstEnd < len(runes) && isASCIIDigit(runes[firstEnd]) {
		firstEnd++
	}
	if firstEnd == 0 || firstEnd >= len(runes) || runes[firstEnd] != '日' {
		return nil, false
	}
	secondStart := firstEnd + 1
	secondEnd := secondStart
	for secondEnd < len(runes) && isASCIIDigit(runes[secondEnd]) {
		secondEnd++
	}
	if secondEnd == secondStart || secondEnd >= len(runes) || runes[secondEnd] != '夜' || secondEnd != len(runes)-1 {
		return nil, false
	}
	parts := numberPinyin(string(runes[:firstEnd]), false)
	parts = append(parts, "tian")
	parts = append(parts, numberPinyin(string(runes[secondStart:secondEnd]), true)...)
	parts = append(parts, "ye")
	return parts, true
}

func isASCIILetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isASCIIDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func numberPinyin(value string, classifier bool) []string {
	if value == "" {
		return nil
	}
	if len(value) == 1 {
		if classifier && value == "2" {
			return []string{"liang"}
		}
		return []string{digitPinyin(rune(value[0]))}
	}
	if len(value) == 2 && value[0] != '0' {
		tens := rune(value[0])
		ones := rune(value[1])
		var parts []string
		if tens == '1' {
			parts = append(parts, "shi")
		} else {
			parts = append(parts, digitPinyin(tens), "shi")
		}
		if ones != '0' {
			parts = append(parts, digitPinyin(ones))
		}
		return parts
	}
	parts := make([]string, 0, len(value))
	for _, r := range value {
		parts = append(parts, digitPinyin(r))
	}
	return parts
}

func digitPinyin(r rune) string {
	switch r {
	case '0':
		return "ling"
	case '1':
		return "yi"
	case '2':
		return "er"
	case '3':
		return "san"
	case '4':
		return "si"
	case '5':
		return "wu"
	case '6':
		return "liu"
	case '7':
		return "qi"
	case '8':
		return "ba"
	case '9':
		return "jiu"
	default:
		return ""
	}
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

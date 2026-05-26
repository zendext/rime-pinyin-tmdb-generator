package pinyin

import "testing"

func TestEncodeReturnsFullPinyinCodes(t *testing.T) {
	got, ok := NewEncoder().Encode("虚构剧集")
	if !ok {
		t.Fatal("expected pinyin encoding to succeed")
	}
	if got != "xu gou ju ji" {
		t.Fatalf("expected xu gou ju ji, got %q", got)
	}
}

func TestEncodeKeepsLatinAcronyms(t *testing.T) {
	got, ok := NewEncoder().Encode("IT狂人")
	if !ok {
		t.Fatal("expected pinyin encoding to succeed")
	}
	if got != "it kuang ren" {
		t.Fatalf("expected it kuang ren, got %q", got)
	}
}

func TestEncodeReadsArabicNumbers(t *testing.T) {
	got, ok := NewEncoder().Encode("9号秘事")
	if !ok {
		t.Fatal("expected pinyin encoding to succeed")
	}
	if got != "jiu hao mi shi" {
		t.Fatalf("expected jiu hao mi shi, got %q", got)
	}
}

func TestEncodeReadsDayNightTitlePattern(t *testing.T) {
	got, ok := NewEncoder().Encode("3日2夜")
	if !ok {
		t.Fatal("expected pinyin encoding to succeed")
	}
	if got != "san tian liang ye" {
		t.Fatalf("expected san tian liang ye, got %q", got)
	}
}

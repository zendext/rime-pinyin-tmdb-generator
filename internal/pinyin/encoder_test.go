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

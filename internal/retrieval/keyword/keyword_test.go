package keyword

import "testing"

func TestTokenizeChineseWithoutSpaces(t *testing.T) {
	tokens := Tokenize("线程和进程有什么区别")
	want := map[string]bool{"线程": false, "进程": false, "区别": false}
	for _, token := range tokens {
		if _, ok := want[token]; ok {
			want[token] = true
		}
	}
	for token, found := range want {
		if !found {
			t.Fatalf("expected token %q in %v", token, tokens)
		}
	}
}

func TestTokenizeMixedLanguage(t *testing.T) {
	tokens := Tokenize("Agent 调用 API")
	got := map[string]bool{}
	for _, token := range tokens {
		got[token] = true
	}
	for _, token := range []string{"agent", "调用", "api"} {
		if !got[token] {
			t.Fatalf("expected token %q in %v", token, tokens)
		}
	}
}

package query

import "testing"

func TestParseUnderstandResult(t *testing.T) {
	result, ok := ParseUnderstandResult(`{"rewrite_query":"进程和线程有什么区别","intent":"kb_search"}`)
	if !ok {
		t.Fatal("expected JSON to parse")
	}
	if result.RewriteQuery != "进程和线程有什么区别" || result.Intent != IntentKBSearch {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseUnderstandResultToleratesWrapper(t *testing.T) {
	result, ok := ParseUnderstandResult("```json\n{\"rewrite_query\":\"你好\",\"intent\":\"greeting\"}\n```")
	if !ok || result.Intent != IntentGreeting {
		t.Fatalf("unexpected result: %+v ok=%v", result, ok)
	}
}

func TestNeedsRetrieval(t *testing.T) {
	cases := []struct {
		intent Intent
		want   bool
	}{
		{IntentGreeting, false},
		{IntentChitchat, false},
		{IntentFollowUp, false},
		{IntentKBSearch, true},
		{IntentClarification, false},
		{IntentSummarize, false},
		{"", true},
	}
	for _, tc := range cases {
		if got := (UnderstandResult{Intent: tc.intent}).NeedsRetrieval(); got != tc.want {
			t.Fatalf("intent=%q got=%v want=%v", tc.intent, got, tc.want)
		}
	}
}

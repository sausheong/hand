package tui

import (
	"testing"
	"time"

	"github.com/sausheong/harness/llm"
)

func TestAddUsage(t *testing.T) {
	acc := llm.Usage{InputTokens: 10, OutputTokens: 5, CacheCreationInputTokens: 1, CacheReadInputTokens: 2}

	got := addUsage(acc, nil)
	if got != acc {
		t.Fatalf("addUsage with nil turn = %+v, want acc unchanged %+v", got, acc)
	}

	got = addUsage(acc, &llm.Usage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 3, CacheReadInputTokens: 4})
	want := llm.Usage{InputTokens: 110, OutputTokens: 25, CacheCreationInputTokens: 4, CacheReadInputTokens: 6}
	if got != want {
		t.Fatalf("addUsage() = %+v, want %+v", got, want)
	}
}

func TestContextTokens(t *testing.T) {
	if got := contextTokens(nil); got != 0 {
		t.Fatalf("contextTokens(nil) = %d, want 0", got)
	}
	u := &llm.Usage{InputTokens: 100, OutputTokens: 9999, CacheCreationInputTokens: 10, CacheReadInputTokens: 20}
	if got := contextTokens(u); got != 130 {
		t.Fatalf("contextTokens() = %d, want 130 (output tokens must be excluded)", got)
	}
}

func TestTotalTokens(t *testing.T) {
	u := llm.Usage{InputTokens: 100, OutputTokens: 50, CacheCreationInputTokens: 10, CacheReadInputTokens: 20}
	if got := totalTokens(u); got != 180 {
		t.Fatalf("totalTokens() = %d, want 180", got)
	}
}

func TestFormatTokenCount(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1500, "1.5k"},
		{12345, "12.3k"},
		{999_999, "1000k"}, // rounds up within the k-suffix branch; still below the 1,000,000 M-threshold check
		{1_000_000, "1M"},
		{2_500_000, "2.5M"},
	}
	for _, tc := range cases {
		if got := formatTokenCount(tc.n); got != tc.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{59 * time.Second, "59.0s"},
		{75 * time.Second, "1m15s"},
	}
	for _, tc := range cases {
		if got := formatDuration(tc.d); got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

package agentio_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
)

// Cancel at a deterministic cooperative checkpoint, after the scanner has had
// enough checks to produce matches. No sleep or machine-speed race is involved.
type searchCheckpointContext struct {
	context.Context
	cancel context.CancelFunc
	checks atomic.Int64
}

func (c *searchCheckpointContext) Err() error {
	if c.checks.Add(1) == 100 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestSearchCancellationDuringScanPreservesPartialMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "many.txt", strings.Repeat("needle\n", 10000))
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &searchCheckpointContext{Context: parent, cancel: cancel}
	result, err := (&agentio.SearchTool{WorkDir: dir}).Execute(ctx, json.RawMessage(`{"content":"needle","max_results":10000,"max_bytes":262144}`))
	if err != nil || result.Error != "" {
		t.Fatal(result, err)
	}
	if parent.Err() != context.Canceled || result.Metadata["cancelled"] != true || result.Metadata["complete"] != false || result.Metadata["truncated"] != false {
		t.Fatal("active cancellation misreported", result.Metadata)
	}
	matches := strings.Count(result.Output, ":needle")
	if matches == 0 || matches >= 10000 || !strings.Contains(result.Output, "many.txt:1:needle") {
		t.Fatal("partial matches missing or scan ignored cancellation", matches)
	}
	if strings.Contains(result.Output, "no matches") || !strings.Contains(result.Output, "cancelled=true") {
		t.Fatal("partial result presented as absence or completion", result.Output)
	}
	// Successful follow-up search uses fresh ownership/context and reaches EOF.
	followup, err := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"absent"}`))
	if err != nil || followup.Metadata["complete"] != true || followup.Metadata["cancelled"] != false {
		t.Fatal("cancelled scan poisoned later search", followup, err)
	}
}

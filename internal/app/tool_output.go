package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sausheong/harness/process"
	"github.com/sausheong/harness/session"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ToolOutput retrieves a result from the selected session history. Display
// truncation cannot change its identity or silently select a different result.
func (c *Controller) ToolOutput(ctx context.Context, sessionID, toolID string) (session.ToolResultData, error) {
	if sessionID == "" || toolID == "" {
		return session.ToolResultData{}, errors.New("tool output identity unavailable")
	}
	if err := ctx.Err(); err != nil {
		return session.ToolResultData{}, err
	}
	for !c.mu.TryRLock() {
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return session.ToolResultData{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		c.mu.RUnlock()
		return session.ToolResultData{}, err
	}
	if c.Rt == nil || c.Rt.Session == nil || c.Rt.Session.ID != sessionID {
		c.mu.RUnlock()
		return session.ToolResultData{}, errors.New("tool output session changed")
	}
	entries := c.Rt.Session.View()
	c.mu.RUnlock()
	var found *session.ToolResultData
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return session.ToolResultData{}, err
		}
		if entry.Type != session.EntryTypeToolResult {
			continue
		}
		var data session.ToolResultData
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			return session.ToolResultData{}, fmt.Errorf("decode tool result: %w", err)
		}
		if data.ToolCallID != toolID {
			continue
		}
		if found != nil {
			return session.ToolResultData{}, errors.New("tool output identity is ambiguous")
		}
		found = &data
	}
	if found == nil {
		return session.ToolResultData{}, errors.New("tool result is not available in selected history")
	}
	return *found, nil
}

// ToolArtifact reads only a persisted reference through the configured capture
// store. It never opens a path extracted from tool output text.
func (c *Controller) ToolArtifact(ctx context.Context, sessionID, toolID, stream string) (string, bool, error) {
	if stream != "stdout" && stream != "stderr" {
		return "", false, errors.New("choose stdout or stderr")
	}
	result, err := c.ToolOutput(ctx, sessionID, toolID)
	if err != nil {
		return "", false, err
	}
	ref, ok := result.Artifacts[stream]
	if !ok {
		return "", false, errors.New("no persisted artifact reference for " + stream)
	}
	store := c.OutputStore
	if store == nil {
		store, err = process.NewArtifactStore(filepath.Join(os.TempDir(), "harness-output-"+strconv.Itoa(os.Getuid())))
		if err != nil {
			return "", false, err
		}
	}
	data, err := store.Read(ctx, ref)
	if err != nil {
		return "", false, err
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", false, errors.New("artifact is binary; text viewer cannot display it")
	}
	return string(data), ref.Truncated, nil
}

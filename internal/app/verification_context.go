package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sausheong/harness/runtime"
)

// retainVerificationContext runs under controller ownership after the evidence
// file is durable. It retains the latest assessment per named profile without
// evicting unrelated task facts. The evidence store retains historical records.
func (c *Controller) retainVerificationContext(ctx context.Context, result VerificationResult) error {
	if c.Rt == nil || c.Rt.Session == nil || result.ID == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(result.Record.Profile))
	id := "verification-" + hex.EncodeToString(hash[:16])
	detail, err := json.Marshal(struct {
		Profile       string `json:"profile"`
		ProfileDigest string `json:"profile_digest"`
		Before        string `json:"before"`
		After         string `json:"after"`
		ExitCode      int    `json:"exit_code"`
		Assessment    string `json:"assessment_at_check"`
		CheckedAt     string `json:"checked_at"`
	}{result.Record.Profile, result.Record.ProfileDigest, result.Record.Before, result.Record.After, result.Record.ExitCode, result.Assessment.Status, time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	item := runtime.ContextStateItem{ID: id, Kind: "verification_reference", Text: string(detail) + " Recheck saved evidence against the current workspace before claiming verification.", Reference: "hand-verification:" + result.ID}
	state, err := c.Rt.ContextState(ctx)
	if err != nil {
		return err
	}
	replaced := false
	for i := range state.Items {
		if state.Items[i].ID != id {
			continue
		}
		if state.Items[i].Kind != "verification_reference" || !strings.HasPrefix(state.Items[i].Reference, "hand-verification:") {
			return errors.New("automatic verification context ID conflicts with an existing task fact")
		}
		state.Items[i] = item
		replaced = true
		break
	}
	if !replaced {
		state.Items = append(state.Items, item)
	}
	if err = c.Rt.SetContextState(ctx, state.Revision, state.Items); err != nil {
		return fmt.Errorf("verification evidence saved as %s, but task context update failed: %w", result.ID, err)
	}
	return nil
}

package app

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sausheong/harness/session"
)

const checkpointJournalKind = "hand.checkpoint"

type CheckpointRecord struct {
	Version int            `json:"version"`
	Phase   string         `json:"phase"`
	Pair    CheckpointPair `json:"pair"`
}
type SessionCheckpoint struct {
	Start  CheckpointPair
	Finish *CheckpointPair
}

func checkpointDigest(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 32 && strings.ToLower(id) == id
}
func validateCheckpointRecord(r CheckpointRecord) error {
	p := r.Pair
	if r.Version != 1 || p.RunID == "" || len(p.RunID) > 128 || !checkpointDigest(p.Before) || len(p.Error) > 4096 {
		return errors.New("invalid checkpoint journal record")
	}
	switch r.Phase {
	case "start":
		if p.After != "" || p.Error != "" {
			return errors.New("checkpoint start has finish data")
		}
	case "finish":
		if (p.After == "" && p.Error == "") || (p.After != "" && !checkpointDigest(p.After)) {
			return errors.New("invalid checkpoint finish")
		}
	default:
		return errors.New("invalid checkpoint phase")
	}
	return nil
}
func ReadSessionCheckpoints(sess *session.Session) ([]SessionCheckpoint, error) {
	if sess == nil {
		return nil, nil
	}
	var result []SessionCheckpoint
	seen := map[string]int{}
	for _, a := range sess.Annotations(checkpointJournalKind) {
		var r CheckpointRecord
		if err := json.Unmarshal(a.Payload, &r); err != nil {
			return nil, err
		}
		if err := validateCheckpointRecord(r); err != nil {
			return nil, err
		}
		i, exists := seen[r.Pair.RunID]
		if r.Phase == "start" {
			if exists {
				return nil, errors.New("duplicate checkpoint start")
			}
			seen[r.Pair.RunID] = len(result)
			result = append(result, SessionCheckpoint{Start: r.Pair})
			continue
		}
		if !exists || result[i].Finish != nil || result[i].Start.Before != r.Pair.Before {
			return nil, errors.New("conflicting checkpoint finish")
		}
		pair := r.Pair
		result[i].Finish = &pair
	}
	return result, nil
}
func (b *HarnessBackend) RecordCheckpoint(phase string, pair CheckpointPair) error {
	if b.Runtime == nil || b.Runtime.Session == nil {
		return errors.New("checkpoint session unavailable")
	}
	// Error prose is diagnostic, not authority; preserve a bounded prefix.
	if len(pair.Error) > 4096 {
		pair.Error = pair.Error[:4096]
	}
	r := CheckpointRecord{1, phase, pair}
	if err := validateCheckpointRecord(r); err != nil {
		return err
	}
	records, err := ReadSessionCheckpoints(b.Runtime.Session)
	if err != nil {
		return err
	}
	found := false
	for _, old := range records {
		if old.Start.RunID != pair.RunID {
			continue
		}
		found = true
		if phase == "start" || old.Finish != nil || old.Start.Before != pair.Before {
			return errors.New("conflicting checkpoint journal record")
		}
	}
	if phase == "finish" && !found {
		return errors.New("checkpoint finish without start")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return b.Runtime.Session.Annotate(checkpointJournalKind, data)
}

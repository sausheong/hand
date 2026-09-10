package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/session"
)

const runJournalKind = "hand.run"

// RunRecord is session metadata, never a prompt or a claim of task correctness.
// ID survives process restarts; RunID is the application-local event identity.
type RunRecord struct {
	Version  int                 `json:"version"`
	BudgetID string              `json:"budget_id,omitempty"`
	ID       string              `json:"id"`
	RunID    uint64              `json:"run_id"`
	Phase    string              `json:"phase"`
	At       time.Time           `json:"at"`
	Model    ModelInfo           `json:"model"`
	Outcome  *agentio.RunOutcome `json:"outcome,omitempty"`
}

// SessionRun without Finish is unfinished or interrupted, never completed.
type SessionRun struct {
	Start  RunRecord
	Finish *RunRecord
}

type runJournal interface{ RecordRun(RunRecord) error }

func validateRunRecord(r RunRecord) error {
	if r.Version != 1 || r.ID == "" || len(r.ID) > 128 || len(r.BudgetID) > 64 || r.RunID == 0 || r.At.IsZero() {
		return errors.New("invalid run journal identity/version/time")
	}
	switch r.Phase {
	case "start":
		if r.Outcome != nil {
			return errors.New("run start has an outcome")
		}
	case "finish":
		if r.Outcome == nil || r.Outcome.Iterations < 0 || r.Outcome.Reason == "" {
			return errors.New("invalid run outcome")
		}
		switch r.Outcome.Status {
		case agentio.Completed, agentio.VerificationFailed, agentio.BudgetExhausted, agentio.Cancelled, agentio.InfrastructureError:
		default:
			return errors.New("unsupported run outcome")
		}
		if r.Outcome.Verified && r.Outcome.Status != agentio.Completed {
			return errors.New("failed run cannot be verified")
		}
	default:
		return errors.New("unsupported run journal phase")
	}
	return nil
}

func (b *HarnessBackend) RecordRun(record RunRecord) error {
	if b.Runtime == nil || b.Runtime.Session == nil {
		return errors.New("run journal session unavailable")
	}
	if err := validateRunRecord(record); err != nil {
		return err
	}
	runs, err := ReadSessionRuns(b.Runtime.Session)
	if err != nil {
		return err
	}
	found := false
	for _, run := range runs {
		if run.Start.ID != record.ID {
			continue
		}
		found = true
		if record.Phase == "start" || run.Finish != nil || run.Start.RunID != record.RunID || run.Start.Model != record.Model || run.Start.BudgetID != record.BudgetID {
			return errors.New("conflicting run journal record")
		}
	}
	if record.Phase == "finish" && !found {
		return errors.New("run finish without start")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return b.Runtime.Session.Annotate(runJournalKind, data)
}

// ReadSessionRuns validates all branches' journal records in durable order.
// An unfinished start remains visible after restart; no finish is inferred.
func ReadSessionRuns(sess *session.Session) ([]SessionRun, error) {
	if sess == nil {
		return nil, nil
	}
	var runs []SessionRun
	ids := map[string]int{}
	for _, annotation := range sess.Annotations(runJournalKind) {
		var record RunRecord
		if err := decodeRunRecord(annotation.Payload, &record); err != nil {
			return nil, err
		}
		if err := validateRunRecord(record); err != nil {
			return nil, err
		}
		index, exists := ids[record.ID]
		if record.Phase == "start" {
			if exists {
				return nil, errors.New("duplicate run start")
			}
			ids[record.ID] = len(runs)
			runs = append(runs, SessionRun{Start: record})
			continue
		}
		if !exists {
			return nil, errors.New("run finish without start")
		}
		if runs[index].Finish != nil || runs[index].Start.RunID != record.RunID || runs[index].Start.Model != record.Model || runs[index].Start.BudgetID != record.BudgetID {
			return nil, fmt.Errorf("conflicting run finish %s", record.ID)
		}
		runs[index].Finish = &record
	}
	return runs, nil
}

func (c *Controller) SessionRuns() ([]SessionRun, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil || c.Rt.Session == nil {
		return nil, errors.New("run journal session unavailable")
	}
	return ReadSessionRuns(c.Rt.Session)
}

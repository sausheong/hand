package sessionio

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

const usageAnnotationKind = "hand.request_usage"

type usageRecord struct {
	Version int              `json:"version"`
	Request llm.RequestUsage `json:"request"`
}

type UsageSummary struct {
	Total             llm.Usage
	Requests, Unknown int
	PriorUsageUnknown bool
}

func RecordRequestUsage(sess *session.Session, request llm.RequestUsage) error {
	if sess == nil {
		return errors.New("usage session unavailable")
	}
	if err := validateUsageRequest(request); err != nil {
		return err
	}
	// The caller holds session ownership through validation and append. Reject
	// conflicts and unreadable journals before adding any durable annotation.
	summary, seen, err := readUsageState(sess)
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if prior, ok := seen[request.ID]; ok {
		if prior != string(canonical) {
			return errors.New("conflicting usage request ID")
		}
		return nil
	}
	if err := addRequestUsage(&summary, request); err != nil {
		return err
	}
	if err := BeginUsageTracking(sess); err != nil {
		return err
	}
	data, err := json.Marshal(usageRecord{Version: 1, Request: request})
	if err != nil {
		return err
	}
	return sess.Annotate(usageAnnotationKind, data)
}

func validateUsageRequest(r llm.RequestUsage) error {
	if r.ID == "" || r.Model == "" {
		return errors.New("usage request identity/model missing")
	}
	if r.Status != "completed" && r.Status != "failed" && r.Status != "cancelled" {
		return errors.New("invalid usage request status")
	}
	if r.Category != llm.CallGeneration && r.Category != llm.CallRetry && r.Category != llm.CallCompaction {
		return errors.New("invalid usage category")
	}
	if r.Usage == nil {
		if r.Source != "unavailable" {
			return errors.New("unknown usage must be unavailable")
		}
		return nil
	}
	u := r.Usage
	if r.Source != "reported" || u.InputTokens < 0 || u.OutputTokens < 0 || u.CacheCreationInputTokens < 0 || u.CacheReadInputTokens < 0 || u.CacheCreationInputTokens > u.InputTokens || u.CacheReadInputTokens > u.InputTokens-u.CacheCreationInputTokens {
		return errors.New("invalid reported usage")
	}
	return nil
}

// ReadUsage counts all attempts in this session, across branches. Duplicate
// identical request IDs are counted once; conflicts and future schemas fail.
// Cached input is a subset of input, never an additional consumption charge.
func ReadUsage(sess *session.Session) (UsageSummary, error) {
	summary, _, err := readUsageState(sess)
	return summary, err
}

func readUsageState(sess *session.Session) (UsageSummary, map[string]string, error) {
	var summary UsageSummary
	seen := make(map[string]string)
	if sess == nil {
		return summary, seen, nil
	}
	known, incomplete, err := readUsageTracking(sess)
	if err != nil {
		return summary, nil, err
	}
	summary.PriorUsageUnknown = incomplete || (!known && len(sess.History()) > 0)
	for _, annotation := range sess.Annotations(usageAnnotationKind) {
		var record usageRecord
		if err := decodeUsageRecord(annotation.Payload, &record); err != nil {
			return summary, nil, err
		}
		if record.Version != 1 {
			return summary, nil, fmt.Errorf("unsupported usage record version %d", record.Version)
		}
		if err := validateUsageRequest(record.Request); err != nil {
			return summary, nil, err
		}
		canonical, _ := json.Marshal(record.Request)
		if previous, ok := seen[record.Request.ID]; ok {
			if previous != string(canonical) {
				return summary, nil, errors.New("conflicting usage request ID")
			}
			continue
		}
		seen[record.Request.ID] = string(canonical)
		if err := addRequestUsage(&summary, record.Request); err != nil {
			return summary, nil, err
		}
	}
	return summary, seen, nil
}

func addRequestUsage(summary *UsageSummary, request llm.RequestUsage) error {
	summary.Requests++
	if request.Usage == nil {
		summary.Unknown++
		return nil
	}
	u := request.Usage
	for _, pair := range []struct {
		target *int
		add    int
	}{
		{&summary.Total.InputTokens, u.InputTokens}, {&summary.Total.OutputTokens, u.OutputTokens},
		{&summary.Total.CacheCreationInputTokens, u.CacheCreationInputTokens}, {&summary.Total.CacheReadInputTokens, u.CacheReadInputTokens},
	} {
		if pair.add > int(^uint(0)>>1)-*pair.target {
			return errors.New("session usage total overflow")
		}
		*pair.target += pair.add
	}
	return nil
}

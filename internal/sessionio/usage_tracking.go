package sessionio

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sausheong/harness/session"
)

const usageTrackingKind = "hand.usage_tracking"

type usageTracking struct {
	Version           int   `json:"version"`
	PriorUsageUnknown *bool `json:"prior_usage_unknown"`
}

func readUsageTracking(sess *session.Session) (bool, bool, error) {
	var known, incomplete bool
	for _, annotation := range sess.Annotations(usageTrackingKind) {
		var record usageTracking
		if err := decodeUsageTracking(annotation.Payload, &record); err != nil {
			return false, false, err
		}
		if record.Version != 1 || record.PriorUsageUnknown == nil {
			return false, false, fmt.Errorf("invalid usage tracking version or payload: %d", record.Version)
		}
		if known && incomplete != *record.PriorUsageUnknown {
			return false, false, errors.New("conflicting usage tracking origins")
		}
		known, incomplete = true, *record.PriorUsageUnknown
	}
	return known, incomplete, nil
}

// BeginUsageTracking must run under session ownership before starting providers.
// Existing history without a tracking origin has unmeasured prior consumption.
// Invoke on an empty fork before copying inherited history: its own accounting
// starts at zero, while the source keeps the cost of producing that history.
func BeginUsageTracking(sess *session.Session) error {
	if sess == nil {
		return errors.New("usage session unavailable")
	}
	known, _, err := readUsageTracking(sess)
	if err != nil || known {
		return err
	}
	incomplete := len(sess.History()) > 0
	data, err := json.Marshal(usageTracking{Version: 1, PriorUsageUnknown: &incomplete})
	if err != nil {
		return err
	}
	return sess.Annotate(usageTrackingKind, data)
}

package app

import "strings"

// Details is an immutable display snapshot. Tool identifiers here are display
// values, never approval capabilities. Truncated input/metadata is not JSON.
// Usage retains the installed Harness version's semantics; it is not a cost
// ledger or a claim that missing usage is zero.
type Details struct {
	ToolPresent                                                               bool
	ToolID, ToolName, ToolInput                                               string
	ResultPresent                                                             bool
	Output, ToolError, Metadata                                               string
	ImageCount                                                                int
	UsagePriorUnknown                                                         bool
	UsageKnown                                                                bool
	UsageRequests, UsageUnknown                                               int
	InputTokens, OutputTokens, CacheCreationInputTokens, CacheReadInputTokens int
	CompactionPresent, Compacted                                              bool
	CompactionReason, Skipped, Summary                                        string
	TurnsCompacted, TokensBefore, TokensAfter                                 int
	DurationMs                                                                int64
	Truncated                                                                 bool
}

// boundDetails caps the aggregate variable-size display payload, including
// identifiers, and clones retained prefixes so the queue cannot retain a large
// backing allocation. The original backend operation is unaffected.
func boundDetails(d Details) Details {
	remaining := MaxEventTextBytes
	for _, field := range []*string{&d.ToolID, &d.ToolName, &d.ToolInput, &d.Output, &d.ToolError, &d.Metadata, &d.CompactionReason, &d.Skipped, &d.Summary} {
		if len(*field) > remaining {
			size := remaining
			for size > 0 && size < len(*field) && ((*field)[size]&0xc0) == 0x80 {
				size--
			}
			*field = (*field)[:size]
			d.Truncated = true
		}
		*field = strings.Clone(*field)
		remaining -= len(*field)
	}
	return d
}

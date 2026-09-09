package app

import (
	"strings"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tokens"
)

// ModelInfo describes requested configuration. An aggregator alias never fills
// ServingModel: that needs separate evidence from the actual serving request.
// These are display values, not approval or model-selection capabilities.
type ModelInfo struct {
	Profile           string
	RequestedModel    string
	ServingModel      string
	ServingModelKnown bool
	ContextLimit      int
	ContextSource     string
	Truncated         bool
}

func runtimeModelInfo(rt *runtime.Runtime, p config.ModelProfile) ModelInfo {
	source := p.ContextSource
	if source == "" {
		source = "runtime_heuristic"
		if rt.ContextWindow > 0 {
			source = "explicit_override"
		}
	}
	return ModelInfo{Profile: p.ProfileName, RequestedModel: rt.Provider + "/" + rt.Model, ContextLimit: tokens.ContextWindowFor(rt.Model, rt.ContextWindow), ContextSource: source}
}
func boundModelInfo(info ModelInfo) ModelInfo {
	remaining := MaxEventTextBytes
	for _, value := range []*string{&info.Profile, &info.RequestedModel, &info.ServingModel, &info.ContextSource} {
		if len(*value) > remaining {
			n := remaining
			for n > 0 && ((*value)[n]&0xc0) == 0x80 {
				n--
			}
			*value = (*value)[:n]
			info.Truncated = true
		}
		*value = strings.Clone(*value)
		remaining -= len(*value)
	}
	return info
}

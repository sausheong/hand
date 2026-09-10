package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"time"
)

type SummarizerOptions struct {
	FollowMain      bool   `json:"follow_main,omitempty"`
	Profile         string `json:"profile"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}
type SummarizerView struct {
	Options             SummarizerOptions `json:"options"`
	Provider            string            `json:"provider"`
	Model               string            `json:"model"`
	Destination         string            `json:"destination"`
	CredentialReference string            `json:"credential_reference,omitempty"`
	Disclosure          string            `json:"disclosure"`
	Digest              string            `json:"digest"`
}

func (c *Controller) summarizerReview(options SummarizerOptions) (SummarizerView, config.ModelProfile, error) {
	var view SummarizerView
	var p config.ModelProfile
	if options.FollowMain {
		if options.Profile != "" {
			return view, p, errors.New("follow_main cannot name a separate profile")
		}
		if c.Rt == nil {
			return view, p, errors.New("runtime unavailable")
		}
		p = config.ModelProfile{Provider: c.Rt.Provider, Model: c.Rt.Model, Endpoint: c.BaseURL, CredentialEnv: c.PermissionProfile.CredentialEnv}
	} else {
		var ok bool
		p, ok = c.profiles[options.Profile]
		if !ok {
			return view, p, errors.New("unknown summariser profile")
		}
	}
	if err := p.Validate(); err != nil {
		return view, p, err
	}
	if p.Reasoning != "" && p.Reasoning != "off" {
		return view, p, errors.New("summariser profile requires reasoning off")
	}
	if options.MaxOutputTokens == 0 {
		options.MaxOutputTokens = 4096
	}
	if options.TimeoutSeconds == 0 {
		options.TimeoutSeconds = 60
	}
	if options.MaxOutputTokens < 1 || options.MaxOutputTokens > 32768 || options.TimeoutSeconds < 1 || options.TimeoutSeconds > 300 {
		return view, p, errors.New("summariser limits require 1-32768 output tokens and 1-300 seconds")
	}
	destination := p.Endpoint
	if destination == "" {
		destination = "default endpoint for " + p.Provider
	}
	view = SummarizerView{Options: options, Provider: p.Provider, Model: p.Model, Destination: destination, CredentialReference: p.CredentialEnv, Disclosure: "Compaction sends session history and pinned guidance to this destination. This selection stays independent of the main model. Output and timeout limits apply per summariser attempt; retries can add usage."}
	if options.FollowMain {
		view.Disclosure = "Compaction sends session history and pinned guidance to the current main-model destination and follows subsequent main-model changes. Output and timeout limits apply per attempt; retries can add usage."
	}
	raw, err := json.Marshal(struct {
		Profile config.ModelProfile
		Options SummarizerOptions
	}{p, options})
	if err != nil {
		return view, p, err
	}
	digest := sha256.Sum256(raw)
	view.Digest = hex.EncodeToString(digest[:])
	return view, copyProfile(p), nil
}
func (c *Controller) ReviewSummarizer(ctx context.Context, options SummarizerOptions) (SummarizerView, error) {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return SummarizerView{}, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	view, _, err := c.summarizerReview(options)
	return view, err
}
func (c *Controller) SelectSummarizer(ctx context.Context, options SummarizerOptions, digest string) error {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt == nil || c.Rt.Compaction == nil {
		return errors.New("compaction manager unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return errors.New("background compaction is still active")
	}
	view, p, err := c.summarizerReview(options)
	if err != nil {
		return err
	}
	if digest == "" || digest != view.Digest {
		return errors.New("summariser review digest does not match")
	}
	provider := c.Rt.LLM
	if !options.FollowMain {
		if c.BuildProfileProvider == nil {
			return errors.New("profile provider factory unavailable")
		}
		provider, err = c.BuildProfileProvider(p)
		if err != nil {
			return fmt.Errorf("build summariser: %w", err)
		}
	}
	if provider == nil {
		return errors.New("summariser factory returned no provider")
	}
	if err = operation.Err(); err != nil {
		return err
	}
	c.Rt.Compaction.Summarizer = &compaction.Summarizer{Route: providerRoute(p.Provider, p.Endpoint), Provider: provider, Model: p.Model, MaxOutputTokens: view.Options.MaxOutputTokens, Timeout: time.Duration(view.Options.TimeoutSeconds) * time.Second}
	if options.FollowMain {
		c.summarizerSelection = nil
	} else {
		c.summarizerSelection = &view
	}
	return nil
}
func (c *Controller) SelectedSummarizer() (SummarizerView, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.summarizerSelection == nil {
		if c.Rt == nil || c.Rt.Compaction == nil || c.Rt.Compaction.Summarizer == nil {
			return SummarizerView{}, false
		}
		summary := c.Rt.Compaction.Summarizer
		view, _, err := c.summarizerReview(SummarizerOptions{FollowMain: true, MaxOutputTokens: summary.MaxOutputTokens, TimeoutSeconds: int(summary.Timeout / time.Second)})
		if err != nil {
			return SummarizerView{}, false
		}
		return view, false
	}
	return *c.summarizerSelection, true
}

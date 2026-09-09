package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DecodePayload rejects duplicate and unknown fields before typed validation.
func DecodePayload(raw json.RawMessage, target any) error {
	if len(raw) > MaxFrameBytes || !utf8.Valid(raw) {
		return errors.New("invalid extension payload size or UTF-8")
	}
	clean := bytes.TrimSpace(raw)
	if len(clean) == 0 || clean[0] != '{' {
		return errors.New("extension payload must be an object")
	}
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target)
}
func displayText(text string, max int, blank bool) bool {
	if len(text) > max || !utf8.ValidString(text) || (!blank && strings.TrimSpace(text) == "") {
		return false
	}
	return strings.IndexFunc(text, func(r rune) bool {
		return (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r)
	}) < 0
}

var capabilityNames = map[string]bool{"commands": true, "lifecycle": true, "context.transform": true, "policy.check": true, "questions": true, "state": true, "presentation": true, "file.read": true, "file.write": true, "network.fetch": true, "process.run": true}

func ValidateCapabilities(capabilities []string) error {
	if len(capabilities) > len(capabilityNames) {
		return errors.New("too many extension capabilities")
	}
	seen := map[string]bool{}
	for _, c := range capabilities {
		if !capabilityNames[c] || seen[c] {
			return fmt.Errorf("unknown or duplicate extension capability %q", c)
		}
		seen[c] = true
	}
	return nil
}

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Hello struct {
	Version       int       `json:"version"`
	Name          string    `json:"name"`
	Capabilities  []string  `json:"capabilities"`
	Commands      []Command `json:"commands,omitempty"`
	Subscriptions []string  `json:"subscriptions,omitempty"`
}

// Validate checks registrations against an immutable host-approved capability
// set. Declarations cannot expand that set and are not an OS sandbox.
func (h Hello) Validate(approved []string) error {
	if h.Version != Version || !identifier(h.Name) {
		return errors.New("invalid extension identity or version")
	}
	if err := ValidateCapabilities(approved); err != nil {
		return err
	}
	if err := ValidateCapabilities(h.Capabilities); err != nil {
		return err
	}
	grants := map[string]bool{}
	declared := map[string]bool{}
	for _, c := range approved {
		grants[c] = true
	}
	for _, c := range h.Capabilities {
		if !grants[c] {
			return fmt.Errorf("extension capability not approved: %s", c)
		}
		declared[c] = true
	}
	if len(h.Commands) > 32 || len(h.Subscriptions) > 5 {
		return errors.New("extension registration limit exceeded")
	}
	if len(h.Commands) > 0 && !declared["commands"] || len(h.Subscriptions) > 0 && !declared["lifecycle"] {
		return errors.New("registration lacks declared capability")
	}
	seen := map[string]bool{}
	for _, c := range h.Commands {
		if !identifier(c.Name) || seen[c.Name] || !displayText(c.Description, 512, false) {
			return errors.New("invalid or duplicate extension command")
		}
		seen[c.Name] = true
	}
	events := map[string]bool{"session.open": true, "session.close": true, "run.start": true, "run.finish": true, "context.compacted": true}
	seen = map[string]bool{}
	for _, event := range h.Subscriptions {
		if !events[event] || seen[event] {
			return errors.New("invalid or duplicate lifecycle subscription")
		}
		seen[event] = true
	}
	return nil
}

// Block is declarative content. No style, escape, outcome, permission or tool
// result override field is accepted. Host rendering must label its provenance.
type Block struct {
	Kind     string   `json:"kind"`
	Text     string   `json:"text,omitempty"`
	Language string   `json:"language,omitempty"`
	Items    []string `json:"items,omitempty"`
}
type Presentation struct {
	Blocks []Block `json:"blocks"`
}

func (p Presentation) Validate() error {
	if len(p.Blocks) > 32 {
		return errors.New("too many extension presentation blocks")
	}
	total := 0
	for _, b := range p.Blocks {
		if !displayText(b.Text, 16<<10, true) {
			return errors.New("unsafe or oversized extension text")
		}
		total += len(b.Text)
		switch b.Kind {
		case "text":
			if b.Language != "" || len(b.Items) > 0 {
				return errors.New("invalid text block fields")
			}
		case "code":
			if len(b.Items) > 0 || (b.Language != "" && !identifier(b.Language)) {
				return errors.New("invalid code block fields")
			}
		case "list":
			if b.Text != "" || b.Language != "" || len(b.Items) > 64 {
				return errors.New("invalid list block")
			}
			for _, item := range b.Items {
				if !displayText(item, 2048, false) {
					return errors.New("unsafe list item")
				}
				total += len(item)
			}
		default:
			return errors.New("unknown extension block kind")
		}
	}
	if total > 64<<10 {
		return errors.New("extension presentation exceeds 64 KiB")
	}
	return nil
}

type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Question struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Options       []Option `json:"options,omitempty"`
	AllowFreeText bool     `json:"allow_free_text"`
}

func (q Question) Validate() error {
	if !identifier(q.ID) || !displayText(q.Title, 2048, false) || len(q.Options) > 12 {
		return errors.New("invalid extension question")
	}
	if len(q.Options) == 0 && !q.AllowFreeText {
		return errors.New("question has no answer mechanism")
	}
	seen := map[string]bool{}
	for _, option := range q.Options {
		if !identifier(option.ID) || seen[option.ID] || !displayText(option.Label, 256, false) {
			return errors.New("invalid question option")
		}
		seen[option.ID] = true
	}
	return nil
}

type Answer struct {
	ID        string `json:"id"`
	Choice    string `json:"choice,omitempty"`
	Text      string `json:"text,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

func (q Question) ValidateAnswer(a Answer) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if a.ID != q.ID {
		return errors.New("question identity mismatch")
	}
	if a.Cancelled {
		if a.Choice != "" || a.Text != "" {
			return errors.New("cancelled answer contains data")
		}
		return nil
	}
	if a.Choice != "" {
		if a.Text != "" {
			return errors.New("ambiguous answer")
		}
		for _, option := range q.Options {
			if option.ID == a.Choice {
				return nil
			}
		}
		return errors.New("unknown option")
	}
	if !q.AllowFreeText || !displayText(a.Text, 4096, false) {
		return errors.New("invalid free-text answer")
	}
	return nil
}

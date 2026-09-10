package checkpoints

import (
	"errors"
	"io/fs"
	"sort"
	"strings"
)

type RestoreAction struct {
	Path      string  `json:"path"`
	Operation string  `json:"operation"`
	Expected  *Record `json:"expected,omitempty"`
	Restore   *Record `json:"restore,omitempty"`
	Conflict  string  `json:"conflict,omitempty"`
}
type RestorePlan struct {
	before, after, current string
	actions                []RestoreAction
}

func (p *RestorePlan) Snapshots() (string, string, string) { return p.before, p.after, p.current }
func (p *RestorePlan) Actions() []RestoreAction {
	result := append([]RestoreAction(nil), p.actions...)
	for i := range result {
		if result[i].Expected != nil {
			v := *result[i].Expected
			result[i].Expected = &v
		}
		if result[i].Restore != nil {
			v := *result[i].Restore
			result[i].Restore = &v
		}
	}
	return result
}
func (p *RestorePlan) HasConflicts() bool {
	for _, a := range p.actions {
		if a.Conflict != "" {
			return true
		}
	}
	return false
}
func omittedAt(s *Snapshot, name string) bool {
	if excluded(name, s.exclusions) {
		return true
	}
	for _, o := range s.omitted {
		if name == o.Path || strings.HasPrefix(name, o.Path+"/") {
			return true
		}
	}
	return false
}
func recordsByPath(s *Snapshot) map[string]Record {
	m := make(map[string]Record, len(s.records))
	for _, r := range s.records {
		m[r.Path] = r
	}
	return m
}

// PlanRestore is a non-mutating, explicit selection preview. It is not an
// execution authorisation: the executor must revalidate the expected file
// identity/content immediately before publishing each change. A snapshot
// cannot prove that an external editor has remained idle since capture.
func PlanRestore(before, after, current *Snapshot, selection []string) (*RestorePlan, error) {
	for _, s := range []*Snapshot{before, after, current} {
		if err := validateSnapshot(s); err != nil {
			return nil, err
		}
	}
	if len(selection) == 0 || len(selection) > 100000 {
		return nil, errors.New("restore requires bounded explicit file selection")
	}
	names := append([]string(nil), selection...)
	sort.Strings(names)
	for i, name := range names {
		if !fs.ValidPath(name) || name == "." || (i > 0 && name == names[i-1]) {
			return nil, errors.New("invalid or duplicate restore path")
		}
	}
	old := recordsByPath(before)
	post := recordsByPath(after)
	now := recordsByPath(current)
	plan := &RestorePlan{before: before.digest, after: after.digest, current: current.digest}
	for _, name := range names {
		a, hadBefore := old[name]
		b, hadAfter := post[name]
		c, hasCurrent := now[name]
		action := RestoreAction{Path: name}
		if hadBefore {
			copy := a
			action.Restore = &copy
		}
		if hadAfter {
			copy := b
			action.Expected = &copy
		}
		switch {
		case omittedAt(before, name) || omittedAt(after, name) || omittedAt(current, name):
			action.Conflict = "path is outside captured restore scope"
		case hadBefore == hadAfter && (!hadBefore || a == b):
			action.Conflict = "path has no recorded change"
		case hasCurrent != hadAfter || (hasCurrent && c != b):
			action.Conflict = "current content or mode differs from recorded after-image"
		}
		switch {
		case !hadBefore:
			action.Operation = "remove"
		case !hadAfter:
			action.Operation = "create"
		default:
			action.Operation = "replace"
		}
		plan.actions = append(plan.actions, action)
	}
	return plan, nil
}

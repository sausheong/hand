package tui

// SetSessionAutoApproval displays the launch-time approval policy. The CLI
// owns the hook; this presentation flag does not change permissions itself.
func (m *Model) SetSessionAutoApproval(enabled bool) { m.sessionAutoApproval = enabled }

func (m *Model) skippingApprovals() bool {
	if m.controller != nil && m.controller.SessionApproval != nil {
		return m.controller.SessionApproval.Skipping()
	}
	return m.sessionAutoApproval
}

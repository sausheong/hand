package tui

// SetInputDraft transfers unsubmitted interactive startup text to the editor.
// It must be called before the main program starts.
func (m *Model) SetInputDraft(text string) {
	m.textarea.CharLimit = 65536
	m.textarea.SetValue(text)
}

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sausheong/hand/extension/protocol"
)

// Extensions supply declarative data, never transcript kinds, approval states,
// terminal styles or Markdown. Keep source blocks so resizing reflows them.
func extensionPresentationBlocks(name string, presentation protocol.Presentation) ([]TranscriptBlock, error) {
	if err := presentation.Validate(); err != nil {
		return nil, err
	}
	blocks := []TranscriptBlock{{Kind: "notice", Text: "Extension " + sanitizeForTerminal(name), Tone: "toolCallStyle"}}
	for _, block := range presentation.Blocks {
		switch block.Kind {
		case "text":
			blocks = append(blocks, TranscriptBlock{Kind: "extension_text", Text: block.Text})
		case "code":
			blocks = append(blocks, TranscriptBlock{Kind: "extension_code", Text: block.Text, Detail: block.Language})
		case "list":
			for _, item := range block.Items {
				blocks = append(blocks, TranscriptBlock{Kind: "extension_item", Text: item})
			}
		}
	}
	return blocks, nil
}

func renderExtensionBlock(block TranscriptBlock, width int) string {
	width = max(1, width)
	text := sanitizeForTerminal(block.Text)
	switch block.Kind {
	case "extension_code":
		label := "code"
		if block.Detail != "" {
			label += " (" + sanitizeForTerminal(block.Detail) + ")"
		}
		if width < 6 {
			return lipgloss.NewStyle().Width(width).Render(label + "\n" + text)
		}
		code := lipgloss.NewStyle().Width(width - 2).Border(lipgloss.NormalBorder()).BorderForeground(colorSlate).Render(text)
		return toolCallStyle.Width(width).Render(label) + "\n" + code
	case "extension_item":
		if width < 3 {
			return lipgloss.NewStyle().Width(width).Render(text)
		}
		wrapped := lipgloss.NewStyle().Width(width - 2).Render(text)
		return "• " + strings.ReplaceAll(wrapped, "\n", "\n  ")
	default:
		return lipgloss.NewStyle().Width(width).Render(text)
	}
}

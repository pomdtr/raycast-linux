package list

import (
	"github.com/charmbracelet/lipgloss"
)

const (
	bullet   = "•"
	ellipsis = "…"
)

// Styles contains style definitions for this list component. By default, these
// values are generated by DefaultStyles.
type Styles struct {
	TitleBar     lipgloss.Style
	Title        lipgloss.Style
	Spinner      lipgloss.Style
	FilterPrompt lipgloss.Style
	FilterCursor lipgloss.Style

	// Default styling for matched characters in a filter. This can be
	// overridden by delegates.
	DefaultFilterCharacterMatch lipgloss.Style

	NoItems lipgloss.Style

	// Styled characters.
	ArabicPagination lipgloss.Style
	DividerDot       lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this list
// component.
func DefaultStyles() (s Styles) {
	verySubduedColor := lipgloss.AdaptiveColor{Light: "#DDDADA", Dark: "#3C3C3C"}
	subduedColor := lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}

	s.TitleBar = lipgloss.NewStyle().Padding(0, 0, 1, 2)

	s.Title = lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1)

	s.FilterPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#ECFD65"})

	s.FilterCursor = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#EE6FF8", Dark: "#EE6FF8"})

	s.DefaultFilterCharacterMatch = lipgloss.NewStyle().Underline(true)

	s.NoItems = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#909090", Dark: "#626262"})

	s.ArabicPagination = lipgloss.NewStyle().Foreground(subduedColor)

	s.DividerDot = lipgloss.NewStyle().
		Foreground(verySubduedColor).
		SetString(" " + bullet + " ")

	return s
}

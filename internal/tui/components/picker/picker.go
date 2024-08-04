package picker

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Represents the message passed when an item from the list is
// selected.
type SelectedMsg struct {
	Item Item
}

// A picker is a component that can be used to show a list of
// values and have the user pick from them.
type Model struct {
	title string
	items []Item

	selected    int
	highlighted int
	focused     bool

	Width  int
	Height int
	Styles Styles
	KeyMap KeyMap
}

func NewModel(title string, items []Item, width int, height int) Model {
	return Model{
		title: title,
		items: items,

		focused:     false,
		selected:    0,
		highlighted: 0,

		Width:  width,
		Height: height,
		Styles: DefaultStyles(),
		KeyMap: DefaultKeyMap(),
	}
}

// Handle the focused-ness of the component.
func (m *Model) Focus() {
	m.focused = true
}

func (m *Model) Blur() {
	m.focused = false
}

func (m Model) IsFocused() bool {
	return m.focused
}

func (m *Model) SetItems(items []Item) {
	m.items = items
	m.selected = 0
	m.highlighted = 0
}

func (m Model) HasItems() bool {
	return len(m.items) > 0
}

func (m Model) Selected() tea.Msg {
	return SelectedMsg{
		Item: m.items[m.selected],
	}
}

func (m Model) clampedIndex(index int) int {
	if index < 0 {
		return 0
	}

	if index >= len(m.items) {
		return len(m.items) - 1
	}

	return index
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.IsFocused() && m.HasItems() {
			switch {
			case key.Matches(msg, m.KeyMap.GoToTop):
				m.highlighted = 0

			case key.Matches(msg, m.KeyMap.GoToLast):
				m.highlighted = len(m.items) - 1

			case key.Matches(msg, m.KeyMap.Up):
				m.highlighted = m.clampedIndex(m.highlighted - 1)

			case key.Matches(msg, m.KeyMap.Down):
				m.highlighted = m.clampedIndex(m.highlighted + 1)

			case key.Matches(msg, m.KeyMap.Select):
				m.selected = m.highlighted
				return m, m.Selected
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	lines := []string{}

	// Add a title to the widget.
	height := m.Height
	if m.title != "" {
		title := m.Styles.Title.Render(m.title)
		lines = append(lines, title)
		height -= lipgloss.Height(title)
	}

	// Keep the highlighted line visible.
	var items []Item
	if m.highlighted > height-1 {
		items = m.items[m.highlighted-height+1 : m.highlighted+1]
	} else {
		if len(m.items) > height {
			items = m.items[:height]
		} else {
			items = m.items
		}
	}

	// Render lines.
	for index, item := range items {
		legend := " "
		if index == m.selected {
			legend = m.Styles.SelectedLegend.Render("┃")
		}

		// If the text overflows, trim it, add ellipsis.
		label := item.Label
		total := len(label) + len(item.Badge) + 2
		if total > m.Width {
			// TODO: Trim from badges too.
			label = label[:len(label)-(total-m.Width+3)] + "…"
		}

		if index == m.highlighted {
			if m.IsFocused() {
				label = m.Styles.Highlighted.Render(label)
			} else {
				if index == m.selected {
					label = m.Styles.SelectedLabel.Render(label)
				} else {
					label = m.Styles.Regular.Render(label)
				}
			}
		} else if index == m.selected {
			label = m.Styles.SelectedLabel.Render(label)
		} else {
			label = m.Styles.Regular.Render(label)
		}

		line := lipgloss.JoinHorizontal(
			lipgloss.Left,
			legend,
			lipgloss.NewStyle().Render(" "),
			label,
		)

		if len(item.Badge) != 0 {
			badge := m.Styles.Badge.Render(item.Badge)
			space := m.Width - lipgloss.Width(line) - lipgloss.Width(badge)
			line = lipgloss.JoinHorizontal(
				lipgloss.Bottom,
				line,
				lipgloss.NewStyle().PaddingRight(space).Render(" "),
				badge,
			)
		}

		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Top, lines...)
}

type Styles struct {
	Title          lipgloss.Style
	Badge          lipgloss.Style
	Regular        lipgloss.Style
	SelectedLabel  lipgloss.Style
	SelectedLegend lipgloss.Style
	Highlighted    lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		Title:          lipgloss.NewStyle().PaddingLeft(2).Height(2).Foreground(lipgloss.Color("244")),
		Badge:          lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		Regular:        lipgloss.NewStyle(),
		SelectedLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		SelectedLegend: lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		Highlighted:    lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
	}
}

type KeyMap struct {
	GoToTop  key.Binding
	GoToLast key.Binding

	Up   key.Binding
	Down key.Binding

	Select key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Select},
		{k.GoToTop, k.GoToLast},
	}
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		GoToTop:  key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "first")),
		GoToLast: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "last")),

		Up:   key.NewBinding(key.WithKeys("k", "up", "ctrl+p"), key.WithHelp("k", "up")),
		Down: key.NewBinding(key.WithKeys("j", "down", "ctrl+n"), key.WithHelp("j", "down")),

		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	}
}

// Represents an item in the picker list.
type Item struct {
	Label string
	Value int
	Badge string
}

func (item *Item) FilterValue() string {
	return item.Label
}

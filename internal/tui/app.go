package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chonamkyu/ssht/internal/config"
	"github.com/chonamkyu/ssht/internal/session"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

type hostItem struct {
	host config.Host
}

func (h hostItem) Title() string       { return h.host.Name }
func (h hostItem) Description() string  { return fmt.Sprintf("%s@%s:%d", h.host.User, h.host.Host, h.host.Port) }
func (h hostItem) FilterValue() string  { return h.host.Name + " " + h.host.Host }

type model struct {
	list     list.Model
	hosts    []config.Host
	quitting bool
	err      error
}

func initialModel(hosts []config.Host) model {
	items := make([]list.Item, len(hosts))
	for i, h := range hosts {
		items[i] = hostItem{host: h}
	}

	l := list.New(items, list.NewDefaultDelegate(), 60, 20)
	l.Title = "ssht - SSH Session Manager"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	return model{
		list:  l,
		hosts: hosts,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(hostItem); ok {
				return m, tea.Sequence(
					tea.ExitAltScreen,
					connectHost(item.host),
				)
			}
		case "b":
			if item, ok := m.list.SelectedItem().(hostItem); ok {
				return m, backgroundConnect(item.host)
			}
		case "s":
			return m, showSessions()
		}
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case connectMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil
	case bgConnectMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	help := statusStyle.Render("  enter: connect | b: background | s: sessions | q: quit")

	if m.err != nil {
		help = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("  Error: %v", m.err))
	}

	return m.list.View() + "\n" + help
}

type connectMsg struct{ err error }
type bgConnectMsg struct{ err error }

func connectHost(host config.Host) tea.Cmd {
	return func() tea.Msg {
		err := session.StartInteractive(&host)
		return connectMsg{err: err}
	}
}

func backgroundConnect(host config.Host) tea.Cmd {
	return func() tea.Msg {
		id, err := session.StartBackground(&host)
		if err != nil {
			return bgConnectMsg{err: err}
		}
		_ = id
		return bgConnectMsg{}
	}
}

func showSessions() tea.Cmd {
	return func() tea.Msg {
		// TODO: switch to sessions view
		return nil
	}
}

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m := initialModel(cfg.Hosts)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err = p.Run()
	return err
}

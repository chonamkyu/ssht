package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chonamkyu/ssht/internal/config"
	"github.com/chonamkyu/ssht/internal/session"
	sshclient "github.com/chonamkyu/ssht/internal/ssh"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	folderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	hostStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	userStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	ipStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	countStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	treeBranch  = dimStyle.Render("├─")
	treeEnd     = dimStyle.Render("└─")
)

type treeItem struct {
	isFolder  bool
	group     string
	host      *config.Host
	collapsed bool
	isLast    bool
	count     int
}

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modeRename
	modeConfirmDelete
)

type model struct {
	items         []treeItem
	hosts         []config.Host
	cursor        int
	showIP        bool
	quitting      bool
	err           error
	mode          mode
	input         textinput.Model
	search        string
	width         int
	height        int
	testResult    map[string]string
	connectTarget *config.Host
	attachID      int // session ID to attach to (0 = new session)
	sessions      map[string]int
}

func buildTree(hosts []config.Host, collapsed map[string]bool) []treeItem {
	groups := map[string][]config.Host{}
	var ungrouped []config.Host
	var groupOrder []string

	for _, h := range hosts {
		g := h.Group
		if g == "" {
			ungrouped = append(ungrouped, h)
		} else {
			if _, exists := groups[g]; !exists {
				groupOrder = append(groupOrder, g)
			}
			groups[g] = append(groups[g], h)
		}
	}

	sort.Strings(groupOrder)

	var items []treeItem
	for _, g := range groupOrder {
		isCollapsed := collapsed[g]
		items = append(items, treeItem{isFolder: true, group: g, collapsed: isCollapsed, count: len(groups[g])})
		if !isCollapsed {
			for i := range groups[g] {
				items = append(items, treeItem{host: &groups[g][i], isLast: i == len(groups[g])-1})
			}
		}
	}
	for i := range ungrouped {
		items = append(items, treeItem{host: &ungrouped[i]})
	}

	return items
}

func initialModel(hosts []config.Host) model {
	collapsed := map[string]bool{}
	items := buildTree(hosts, collapsed)

	sessMap := map[string]int{}
	if infos, err := session.List(); err == nil {
		for _, s := range infos {
			sessMap[s.HostName] = s.ID
		}
	}

	return model{
		items:      items,
		hosts:      hosts,
		cursor:     0,
		width:      80,
		height:     24,
		testResult: map[string]string{},
		sessions:   sessMap,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSearch:
		return m.updateSearch(msg)
	case modeRename:
		return m.updateRename(msg)
	case modeConfirmDelete:
		return m.updateConfirmDelete(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			item := m.items[m.cursor]
			if item.isFolder {
				m.toggleFolder()
			} else if item.host != nil {
				m.connectTarget = item.host
				m.attachID = 0
				return m, tea.Quit
			}
		case "l", "right":
			item := m.items[m.cursor]
			if item.isFolder {
				m.toggleFolder()
			}
		case "c":
			item := m.items[m.cursor]
			if !item.isFolder && item.host != nil {
				if sid, ok := m.sessions[item.host.Name]; ok {
					m.connectTarget = item.host
					m.attachID = sid
					return m, tea.Quit
				} else {
					m.err = fmt.Errorf("no active session for %s", item.host.Name)
				}
			}
		case "h", "left":
			item := m.items[m.cursor]
			if item.isFolder && !item.collapsed {
				m.toggleFolder()
			}
		case " ":
			m.toggleFolder()
		case "b":
			item := m.items[m.cursor]
			if !item.isFolder && item.host != nil {
				return m, backgroundConnect(*item.host)
			}
		case "i", "tab":
			m.showIP = !m.showIP
		case "?":
			m.mode = modeSearch
			m.input = textinput.New()
			m.input.Placeholder = "search..."
			m.input.Focus()
			return m, m.input.Cursor.BlinkCmd()
		case "r":
			item := m.items[m.cursor]
			if item.isFolder {
				m.mode = modeRename
				m.input = textinput.New()
				m.input.Prompt = "RenameGroup: "
				m.input.SetValue(item.group)
				m.input.Focus()
				return m, m.input.Cursor.BlinkCmd()
			} else if item.host != nil && item.host.Source != "ssh_config" {
				m.mode = modeRename
				m.input = textinput.New()
				m.input.SetValue(item.host.Name)
				m.input.Focus()
				return m, m.input.Cursor.BlinkCmd()
			}
		case "d":
			item := m.items[m.cursor]
			if !item.isFolder && item.host != nil && item.host.Source != "ssh_config" {
				m.mode = modeConfirmDelete
			}
		case "g":
			m.moveToGroup()
		case "t":
			item := m.items[m.cursor]
			if !item.isFolder && item.host != nil {
				return m, testConnection(*item.host)
			}
		case "K":
			item := m.items[m.cursor]
			if !item.isFolder && item.host != nil {
				if sid, ok := m.sessions[item.host.Name]; ok {
					session.Kill(sid)
					delete(m.sessions, item.host.Name)
				}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case bgConnectMsg:
		if msg.err != nil {
			m.err = msg.err
		}
	case testMsg:
		if msg.err != nil {
			m.testResult[msg.host] = "fail"
		} else {
			m.testResult[msg.host] = "ok"
		}
	}

	return m, nil
}

func (m *model) toggleFolder() {
	item := m.items[m.cursor]
	if !item.isFolder {
		return
	}

	collapsed := m.getCollapsedMap()
	collapsed[item.group] = !item.collapsed
	m.items = buildTree(m.hosts, collapsed)

	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
}

func (m *model) getCollapsedMap() map[string]bool {
	collapsed := map[string]bool{}
	for _, item := range m.items {
		if item.isFolder {
			collapsed[item.group] = item.collapsed
		}
	}
	return collapsed
}

func (m *model) deleteSelected() {
	item := m.items[m.cursor]
	if item.isFolder || item.host == nil || item.host.Source == "ssh_config" {
		return
	}

	cfg, err := config.Load()
	if err != nil {
		return
	}

	if err := cfg.RemoveHost(item.host.Name); err != nil {
		return
	}
	cfg.Save()

	newHosts := []config.Host{}
	for _, h := range m.hosts {
		if h.Name != item.host.Name {
			newHosts = append(newHosts, h)
		}
	}
	m.hosts = newHosts
	m.items = buildTree(m.hosts, m.getCollapsedMap())

	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
}

func (m *model) moveToGroup() {
	item := m.items[m.cursor]
	if item.isFolder || item.host == nil || item.host.Source == "ssh_config" {
		return
	}

	m.mode = modeRename
	m.input = textinput.New()
	m.input.Placeholder = "group name (empty to ungroup)"
	m.input.SetValue(item.host.Group)
	m.input.Focus()
	// re-use rename mode but tag it
	m.input.Prompt = "Group: "
}

func (m model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.deleteSelected()
			m.mode = modeNormal
		case "n", "N", "esc":
			m.mode = modeNormal
		}
	}
	return m, nil
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			m.mode = modeNormal
			m.search = m.input.Value()
			if msg.String() == "esc" {
				m.search = ""
			}
			m.rebuildFiltered()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.search = m.input.Value()
	m.rebuildFiltered()
	return m, cmd
}

func (m *model) rebuildFiltered() {
	if m.search == "" {
		m.items = buildTree(m.hosts, m.getCollapsedMap())
		return
	}

	query := strings.ToLower(m.search)
	var filtered []config.Host
	for _, h := range m.hosts {
		match := strings.Contains(strings.ToLower(h.Name), query) ||
			strings.Contains(strings.ToLower(h.Host), query) ||
			strings.Contains(strings.ToLower(h.User), query) ||
			strings.Contains(strings.ToLower(h.Group), query)
		if match {
			filtered = append(filtered, h)
		}
	}
	m.items = buildTree(filtered, map[string]bool{})
	m.cursor = 0
}

func (m model) updateRename(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			item := m.items[m.cursor]

			cfg, err := config.Load()
			if err != nil {
				m.mode = modeNormal
				return m, nil
			}

			if m.input.Prompt == "RenameGroup: " {
				oldGroup := item.group
				newGroup := m.input.Value()
				if newGroup != "" && newGroup != oldGroup {
					for i := range m.hosts {
						if m.hosts[i].Group == oldGroup {
							m.hosts[i].Group = newGroup
						}
					}
					for i := range cfg.Hosts {
						if cfg.Hosts[i].Group == oldGroup {
							cfg.Hosts[i].Group = newGroup
						}
					}
					cfg.Save()
				}
			} else if m.input.Prompt == "Group: " {
				if item.host != nil {
					for i, h := range m.hosts {
						if h.Name == item.host.Name {
							m.hosts[i].Group = m.input.Value()
							break
						}
					}
					h, _ := cfg.FindHost(item.host.Name)
					if h != nil {
						h.Group = m.input.Value()
						cfg.Save()
					}
				}
			} else {
				if item.host != nil {
					newName := m.input.Value()
					if newName != "" {
						oldName := item.host.Name
						if err := cfg.RenameHost(oldName, newName); err == nil {
							cfg.Save()
							for i, h := range m.hosts {
								if h.Name == oldName {
									m.hosts[i].Name = newName
									break
								}
							}
						}
					}
				}
			}

			m.items = buildTree(m.hosts, m.getCollapsedMap())
			m.mode = modeNormal
			return m, nil
		case "esc":
			m.mode = modeNormal
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("ssht"))
	b.WriteString("\n\n")

	visibleHeight := m.height - 5
	start := 0
	if m.cursor >= visibleHeight {
		start = m.cursor - visibleHeight + 1
	}

	end := start + visibleHeight
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := start; i < end; i++ {
		item := m.items[i]
		selected := i == m.cursor

		if item.isFolder {
			icon := "📂"
			arrow := " ▾"
			if item.collapsed {
				icon = "📁"
				arrow = " ▸"
			}
			label := icon + " " + folderStyle.Render(item.group) + arrow + " " + countStyle.Render(fmt.Sprintf("(%d)", item.count))
			if selected {
				b.WriteString(selStyle.Render(" ❯ ") + label + "\n")
			} else {
				b.WriteString("   " + label + "\n")
			}
		} else if item.host != nil {
			inGroup := item.host.Group != ""

			name := hostStyle.Render(item.host.Name)
			detail := userStyle.Render(item.host.User)
			if m.showIP {
				detail += "  " + ipStyle.Render(fmt.Sprintf("%s:%d", item.host.Host, item.host.Port))
			}

			if sid, hasSess := m.sessions[item.host.Name]; hasSess {
				detail += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render(fmt.Sprintf("session::%d", sid))
			}

			if result, ok := m.testResult[item.host.Name]; ok {
				if result == "ok" {
					detail += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● ok")
				} else {
					detail += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("● fail")
				}
			}

			if selected {
				name = selStyle.Render(item.host.Name)
			}

			var line string
			if inGroup {
				branch := treeBranch
				if item.isLast {
					branch = treeEnd
				}
				if selected {
					line = selStyle.Render(" ❯") + "    " + branch + " " + name + "  " + detail
				} else {
					line = "      " + branch + " " + name + "  " + detail
				}
			} else {
				if selected {
					line = selStyle.Render(" ❯") + " " + name + "  " + detail
				} else {
					line = "   " + name + "  " + detail
				}
			}

			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n")

	switch m.mode {
	case modeSearch:
		b.WriteString("  Search: " + m.input.View())
	case modeRename:
		label := "Rename"
		if m.input.Prompt == "Group: " {
			label = "Group"
		} else if m.input.Prompt == "RenameGroup: " {
			label = "Rename Group"
		}
		b.WriteString(fmt.Sprintf("  %s: %s  (enter: confirm, esc: cancel)", label, m.input.View()))
	case modeConfirmDelete:
		item := m.items[m.cursor]
		name := ""
		if item.host != nil {
			name = item.host.Name
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(
			fmt.Sprintf("  Delete '%s'? (y/n)", name)))
	default:
		if m.err != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("  %v", m.err)))
		} else {
			b.WriteString(helpStyle.Render("  enter: new session | c: attach | b: bg | K: kill | r: rename | d: del | g: group | i: IP | t: test | ?: search | q: quit"))
		}
	}

	return b.String()
}

type bgConnectMsg struct{ err error }
type testMsg struct {
	host string
	err  error
}

func testConnection(host config.Host) tea.Cmd {
	return func() tea.Msg {
		client, err := sshclient.Connect(&host)
		if err != nil {
			return testMsg{host: host.Name, err: err}
		}
		client.Close()
		return testMsg{host: host.Name}
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

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m := initialModel(cfg.Hosts)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		return err
	}

	if final, ok := result.(model); ok && final.connectTarget != nil {
		if final.attachID > 0 {
			return session.Attach(final.attachID)
		}
		return session.StartInteractive(final.connectTarget)
	}

	return nil
}

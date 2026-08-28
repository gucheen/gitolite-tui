package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gucheng01/gitolite-tui/internal/gitolite"
	"github.com/gucheng01/gitolite-tui/internal/repository"
)

type Model struct {
	client gitolite.Client
	store  repository.Store

	repos       []gitolite.Repository
	filtered    []int
	cursor      int
	query       string
	search      bool
	commits     []repository.Commit
	active      string
	status      string
	err         error
	loading     bool
	width       int
	height      int
	trash       []string
	trashCursor int
	showTrash   bool
	prompt      promptKind
	input       string
	target      string
}

type promptKind int

const (
	promptNone promptKind = iota
	promptCreate
	promptDescription
	promptTrash
	promptRestore
)

type reposMsg struct {
	repos  []gitolite.Repository
	status string
	err    error
}

type trashMsg struct {
	entries []string
	status  string
	err     error
}

type descriptionMsg struct {
	repo        string
	description string
	err         error
}

type operationMsg struct {
	action string
	target string
	err    error
}

type logMsg struct {
	repo    string
	commits []repository.Commit
	err     error
	status  string
}

type statusMsg struct {
	text string
	err  error
}

type openTigMsg struct {
	cmd *exec.Cmd
	err error
}

type cloneRequestMsg struct {
	cmd         *exec.Cmd
	destination string
	err         error
}

func New(client gitolite.Client, store repository.Store) Model {
	return Model{client: client, store: store, loading: true, status: "Loading repositories…"}
}

func (m Model) Init() tea.Cmd {
	return m.loadReposCmd()
}

func (m Model) loadReposCmd() tea.Cmd {
	return m.loadReposWithStatusCmd("")
}

func (m Model) loadReposWithStatusCmd(status string) tea.Cmd {
	return func() tea.Msg {
		repos, err := m.client.List(context.Background())
		return reposMsg{repos: repos, status: status, err: err}
	}
}

func (m Model) loadTrashCmd(status string) tea.Cmd {
	return func() tea.Msg {
		entries, err := m.client.ListTrash(context.Background())
		return trashMsg{entries: entries, status: status, err: err}
	}
}

func (m Model) createRepoCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.Create(context.Background(), repo)
		return operationMsg{action: "create", target: repo, err: err}
	}
}

func (m Model) loadDescriptionCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		description, err := m.client.Description(context.Background(), repo)
		return descriptionMsg{repo: repo, description: description, err: err}
	}
}

func (m Model) setDescriptionCmd(repo, description string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.SetDescription(context.Background(), repo, description)
		return operationMsg{action: "describe", target: repo, err: err}
	}
}

func (m Model) trashRepoCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.Trash(context.Background(), repo)
		return operationMsg{action: "trash", target: repo, err: err}
	}
}

func (m Model) restoreRepoCmd(trashID string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.Restore(context.Background(), trashID)
		return operationMsg{action: "restore", target: trashID, err: err}
	}
}

func (m Model) selected() (gitolite.Repository, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return gitolite.Repository{}, false
	}
	return m.repos[m.filtered[m.cursor]], true
}

func (m Model) selectedTrash() (string, bool) {
	if m.trashCursor < 0 || m.trashCursor >= len(m.trash) {
		return "", false
	}
	return m.trash[m.trashCursor], true
}

func (m *Model) closePrompt() {
	m.prompt = promptNone
	m.input = ""
	m.target = ""
}

func (m *Model) applyFilter() {
	m.filtered = m.filtered[:0]
	needle := strings.ToLower(strings.TrimSpace(m.query))
	for i, repo := range m.repos {
		if needle == "" || strings.Contains(strings.ToLower(repo.Name), needle) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m Model) loadLogCmd(repo string, refresh bool) tea.Cmd {
	return func() tea.Msg {
		url := m.client.CloneURL(repo)
		if err := m.store.Ensure(context.Background(), repo, url); err != nil {
			return logMsg{repo: repo, err: err}
		}
		status := "Loaded cached repository"
		if refresh {
			if err := m.store.Refresh(context.Background(), repo); err != nil {
				return logMsg{repo: repo, err: err}
			}
			status = "Refreshed cached repository"
		}
		commits, err := m.store.Log(context.Background(), repo)
		return logMsg{repo: repo, commits: commits, err: err, status: status}
	}
}

func (m Model) cloneCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		name := strings.TrimSuffix(filepath.Base(repo), ".git")
		if name == "" || name == "." {
			return cloneRequestMsg{err: fmt.Errorf("cannot derive clone directory from %q", repo)}
		}
		destination := filepath.Join(".", name)
		absolute, err := filepath.Abs(destination)
		if err != nil {
			return cloneRequestMsg{err: fmt.Errorf("resolve clone directory: %w", err)}
		}
		return cloneRequestMsg{cmd: m.store.CloneCommand(m.client.CloneURL(repo), destination), destination: absolute}
	}
}

func (m Model) copyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if err := copyClipboard(text); err != nil {
			return statusMsg{err: err}
		}
		return statusMsg{text: "Clone URL copied"}
	}
}

func (m Model) prepareTigCmd(repo string) tea.Cmd {
	return func() tea.Msg {
		cmd, err := m.store.TigCommand(repo)
		return openTigMsg{cmd: cmd, err: err}
	}
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.closePrompt()
		m.err = nil
		m.status = "Cancelled"
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if m.prompt != promptRestore && m.input != "" {
			_, size := utf8.DecodeLastRuneInString(m.input)
			m.input = m.input[:len(m.input)-size]
		}
		return m, nil
	case tea.KeyRunes:
		if m.prompt != promptRestore {
			m.input += string(msg.Runes)
		}
		return m, nil
	case tea.KeySpace:
		if m.prompt == promptDescription {
			m.input += " "
		}
		return m, nil
	case tea.KeyEnter:
		switch m.prompt {
		case promptCreate:
			repo := strings.TrimSpace(m.input)
			if repo == "" {
				m.err = errors.New("repository name cannot be empty")
				return m, nil
			}
			m.closePrompt()
			m.loading, m.err, m.status = true, nil, "Creating "+repo+"…"
			return m, m.createRepoCmd(repo)
		case promptDescription:
			description := strings.TrimSpace(m.input)
			if description == "" {
				m.err = errors.New("description cannot be empty")
				return m, nil
			}
			repo := m.target
			m.closePrompt()
			m.loading, m.err, m.status = true, nil, "Updating description for "+repo+"…"
			return m, m.setDescriptionCmd(repo, description)
		case promptTrash:
			if strings.TrimSpace(m.input) != m.target {
				m.err = errors.New("repository name does not match")
				return m, nil
			}
			repo := m.target
			m.closePrompt()
			m.loading, m.err, m.status = true, nil, "Moving "+repo+" to trash…"
			return m, m.trashRepoCmd(repo)
		case promptRestore:
			trashID := m.target
			m.closePrompt()
			m.loading, m.err, m.status = true, nil, "Restoring "+trashID+"…"
			return m, m.restoreRepoCmd(trashID)
		}
	}
	return m, nil
}

func (m Model) updateTrash(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "T":
		m.showTrash = false
		m.loading, m.err, m.status = true, nil, "Reloading repository list…"
		return m, m.loadReposCmd()
	case "up", "k":
		if m.trashCursor > 0 {
			m.trashCursor--
		}
	case "down", "j":
		if m.trashCursor+1 < len(m.trash) {
			m.trashCursor++
		}
	case "pgup":
		m.trashCursor = max(0, m.trashCursor-max(1, m.layout().listRows))
	case "pgdown":
		m.trashCursor = min(max(0, len(m.trash)-1), m.trashCursor+max(1, m.layout().listRows))
	case "home":
		m.trashCursor = 0
	case "end":
		m.trashCursor = max(0, len(m.trash)-1)
	case "R":
		m.loading, m.err, m.status = true, nil, "Reloading trash…"
		return m, m.loadTrashCmd("")
	case "r", "enter":
		if trashID, ok := m.selectedTrash(); ok {
			m.prompt = promptRestore
			m.target = trashID
			m.err = nil
			m.status = "Confirm restore"
		}
	}
	return m, nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case reposMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.repos = msg.repos
			m.applyFilter()
			if msg.status != "" {
				m.status = msg.status
			} else {
				m.status = fmt.Sprintf("Found %d repositories", len(msg.repos))
			}
		}
	case trashMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.trash = msg.entries
			if m.trashCursor >= len(m.trash) {
				m.trashCursor = max(0, len(m.trash)-1)
			}
			if msg.status != "" {
				m.status = msg.status
			} else {
				m.status = fmt.Sprintf("Found %d repositories in trash", len(msg.entries))
			}
		}
	case descriptionMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.prompt = promptDescription
			m.input = msg.description
			m.target = msg.repo
			m.status = "Editing description for " + msg.repo
		}
	case operationMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			break
		}
		switch msg.action {
		case "create":
			m.loading = true
			return m, m.loadReposWithStatusCmd("Created " + msg.target)
		case "trash":
			m.loading = true
			m.active, m.commits = "", nil
			return m, m.loadReposWithStatusCmd("Moved " + msg.target + " to trash")
		case "restore":
			m.loading = true
			return m, m.loadTrashCmd("Restored " + msg.target)
		case "describe":
			m.status = "Updated description for " + msg.target
		}
	case logMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.active = msg.repo
			m.commits = msg.commits
			m.status = fmt.Sprintf("%s; %d commits shown", msg.status, len(msg.commits))
		}
	case statusMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.status = msg.text
		}
	case openTigMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			break
		}
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			if err != nil {
				return statusMsg{err: fmt.Errorf("tig: %w", err)}
			}
			return statusMsg{text: "Returned from tig"}
		})
	case cloneRequestMsg:
		if msg.err != nil {
			m.loading = false
			m.err = msg.err
			break
		}
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			if err != nil {
				return statusMsg{err: fmt.Errorf("git clone: %w", err)}
			}
			return statusMsg{text: "Cloned to " + msg.destination}
		})
	case tea.KeyMsg:
		if m.prompt != promptNone {
			return m.updatePrompt(msg)
		}
		if m.showTrash {
			return m.updateTrash(msg)
		}
		if m.search {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyEnter:
				m.search = false
			case tea.KeyBackspace, tea.KeyDelete:
				if m.query != "" {
					_, size := utf8.DecodeLastRuneInString(m.query)
					m.query = m.query[:len(m.query)-size]
					m.cursor = 0
					m.applyFilter()
				}
			case tea.KeyRunes:
				m.query += string(msg.Runes)
				m.cursor = 0
				m.applyFilter()
			case tea.KeyCtrlC:
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "n":
			m.prompt = promptCreate
			m.input, m.target, m.err = "", "", nil
			m.status = "Enter a wildcard repository name"
		case "T":
			m.showTrash = true
			m.trashCursor = 0
			m.loading, m.err, m.status = true, nil, "Loading trash…"
			return m, m.loadTrashCmd("")
		case "d":
			if repo, ok := m.selected(); ok {
				if repo.Wildcard {
					m.err = nil
					m.status = "Wildcard repository rules cannot be moved to trash"
					break
				}
				m.prompt = promptTrash
				m.input, m.target, m.err = "", repo.Name, nil
				m.status = "Type the full repository name to confirm"
			}
		case "e":
			if repo, ok := m.selected(); ok {
				if repo.Wildcard {
					m.err = nil
					m.status = "Wildcard repository rules do not have descriptions"
					break
				}
				m.loading, m.err, m.status = true, nil, "Loading description for "+repo.Name+"…"
				return m, m.loadDescriptionCmd(repo.Name)
			}
		case "/":
			m.search = true
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor+1 < len(m.filtered) {
				m.cursor++
			}
		case "pgup":
			m.cursor = max(0, m.cursor-max(1, m.layout().listRows))
		case "pgdown":
			m.cursor = min(max(0, len(m.filtered)-1), m.cursor+max(1, m.layout().listRows))
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = max(0, len(m.filtered)-1)
		case "esc":
			m.active, m.commits = "", nil
		case "enter":
			if repo, ok := m.selected(); ok {
				if repo.Wildcard {
					m.err, m.active, m.commits = nil, "", nil
					m.status = "Wildcard repository rules do not have commit logs"
					break
				}
				m.loading, m.err, m.status = true, nil, "Caching and loading "+repo.Name+"…"
				return m, m.loadLogCmd(repo.Name, false)
			}
		case "r":
			if repo, ok := m.selected(); ok {
				if repo.Wildcard {
					m.err, m.active, m.commits = nil, "", nil
					m.status = "Wildcard repository rules cannot be refreshed"
					break
				}
				m.loading, m.err, m.status = true, nil, "Refreshing "+repo.Name+"…"
				return m, m.loadLogCmd(repo.Name, true)
			}
		case "R":
			m.loading, m.err, m.status = true, nil, "Reloading repository list…"
			return m, m.loadReposCmd()
		case "c":
			if repo, ok := m.selected(); ok {
				return m, m.copyCmd(m.client.CloneURL(repo.Name))
			}
		case "l":
			if repo, ok := m.selected(); ok {
				m.loading, m.err, m.status = true, nil, "Cloning "+repo.Name+"…"
				return m, m.cloneCmd(repo.Name)
			}
		case "t":
			if repo, ok := m.selected(); ok {
				if repo.Wildcard {
					m.err = nil
					m.status = "Wildcard repository rules cannot be opened with tig"
					break
				}
				m.loading, m.err, m.status = true, nil, "Opening tig…"
				return m, m.prepareTigCmd(repo.Name)
			}
		}
	}
	return m, nil
}

func (m Model) promptLines(width int) []string {
	switch m.prompt {
	case promptCreate:
		return []string{"", "Create wildcard repository", inputLine("Repository: "+m.input, width), "enter create  esc cancel"}
	case promptDescription:
		return []string{"", "Edit description — " + m.target, inputLine("Description: "+m.input, width), "enter save  esc cancel"}
	case promptTrash:
		return []string{"", "Move " + m.target + " to trash?", inputLine("Type the full repository name: "+m.input, width), "enter trash  esc cancel"}
	case promptRestore:
		return []string{"", "Restore " + m.target + "?", "enter restore  esc cancel"}
	}
	return nil
}

func inputLine(value string, width int) string {
	value = singleLine(value) + "█"
	if cells := ansi.StringWidth(value); cells > width {
		return "…" + ansi.Cut(value, cells-width+1, cells)
	}
	return value
}

type viewLayout struct {
	width, height          int
	header, detail, footer []string
	listRows, commitRows   int
}

func (m Model) layout() viewLayout {
	l := viewLayout{width: m.width, height: m.height}
	if l.width <= 0 {
		l.width = 80
	}
	if l.height <= 0 {
		l.height = 24
	}
	l.header = []string{fmt.Sprintf("gitolite-tui  %s@%s", m.client.User, m.client.Host)}
	help := []string{"↑/↓ select", "pgup/pgdown page", "home/end jump"}
	if m.showTrash {
		l.header = append(l.header, "Trash")
		help = append(help, "enter/r restore", "R reload", "T/esc repositories", "q quit")
	} else {
		search := "Search: " + m.query + "  (press / to edit)"
		if m.search {
			search = inputLine("Search: "+m.query, l.width)
		}
		l.header = append(l.header, search)
		help = append(help, "enter log", "esc hide log", "n new", "e desc", "d trash", "T trash-bin", "/ search",
			"c copy", "l clone", "r refresh", "R reload", "t tig", "q quit")
		if repo, ok := m.selected(); ok {
			l.detail = []string{"", "Clone: " + m.client.CloneURL(repo.Name)}
			if repo.Wildcard {
				l.detail = append(l.detail, "This is a wildcard rule; log, refresh, and tig are unavailable.")
			}
			if m.active == repo.Name {
				l.detail = append(l.detail, "Recent commits — "+m.active)
				l.commitRows = len(m.commits)
			}
		}
	}
	l.header = append(l.header, strings.Repeat("─", l.width))
	l.footer = m.promptLines(l.width)
	status := m.status
	if m.err != nil {
		status = fmt.Sprintf("Error: %v", m.err)
	}
	l.footer = append(l.footer, "", status)
	line := ""
	for _, binding := range help {
		if line != "" && ansi.StringWidth(line)+2+ansi.StringWidth(binding) > l.width {
			l.footer = append(l.footer, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += binding
	}
	l.footer = append(l.footer, line)
	available := max(0, l.height-len(l.header)-len(l.detail)-len(l.footer))
	l.commitRows = min(l.commitRows, available/3)
	l.listRows = available - l.commitRows
	return l
}

func (m Model) View() string {
	l := m.layout()
	if l.listRows == 0 || l.width < 20 {
		lines := []string{"Terminal too small; enlarge to continue.", "ctrl+c quit"}
		for i := range lines {
			lines[i] = truncate(lines[i], l.width)
		}
		return strings.Join(lines[:min(len(lines), l.height)], "\n")
	}

	lines := l.header
	count, cursor := len(m.filtered), m.cursor
	if m.showTrash {
		count, cursor = len(m.trash), m.trashCursor
	}
	start := min(max(0, cursor-l.listRows+1), max(0, count-l.listRows))
	for row := start; row < start+l.listRows; row++ {
		line := ""
		if row < count {
			marker := "  "
			if row == cursor {
				marker = "> "
			}
			if m.showTrash {
				line = marker + m.trash[row]
			} else {
				repo := m.repos[m.filtered[row]]
				line = fmt.Sprintf("%s%-6s %s", marker, repo.Access, repo.Name)
				if repo.Wildcard {
					line += "  [wildcard]"
				}
			}
		} else if count == 0 && row == 0 && !m.loading {
			line = "  No repositories found"
			if m.showTrash {
				line = "  Trash is empty"
			}
		}
		lines = append(lines, line)
	}
	lines = append(lines, l.detail...)
	for _, commit := range m.commits[:l.commitRows] {
		lines = append(lines, fmt.Sprintf("%s %s %-14s %s", commit.Hash, commit.Date, commit.Author, commit.Subject))
	}
	lines = append(lines, l.footer...)
	for i := range lines {
		lines[i] = truncate(lines[i], l.width)
	}
	return strings.Join(lines, "\n")
}

func copyClipboard(value string) error {
	type candidate struct {
		name string
		args []string
	}
	candidates := []candidate{{name: "wl-copy"}, {name: "xclip", args: []string{"-selection", "clipboard"}}, {name: "xsel", args: []string{"--clipboard", "--input"}}}
	if runtime.GOOS == "darwin" {
		candidates = []candidate{{name: "pbcopy"}}
	}
	for _, item := range candidates {
		path, err := exec.LookPath(item.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, item.args...)
		cmd.Stdin = strings.NewReader(value)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("copy with %s: %w: %s", item.name, err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	return fmt.Errorf("no clipboard command found")
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(singleLine(value), width, "…")
}

func singleLine(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

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

func (m Model) writePrompt(out *strings.Builder) {
	switch m.prompt {
	case promptCreate:
		out.WriteString("\nCreate wildcard repository\nRepository: " + m.input + "█\n")
		out.WriteString("enter create  esc cancel\n")
	case promptDescription:
		fmt.Fprintf(out, "\nEdit description — %s\nDescription: %s█\n", m.target, m.input)
		out.WriteString("enter save  esc cancel\n")
	case promptTrash:
		fmt.Fprintf(out, "\nMove %s to trash?\nType the full repository name: %s█\n", m.target, m.input)
		out.WriteString("enter trash  esc cancel\n")
	case promptRestore:
		fmt.Fprintf(out, "\nRestore %s?\n", m.target)
		out.WriteString("enter restore  esc cancel\n")
	}
}

func (m Model) viewTrash() string {
	var out strings.Builder
	fmt.Fprintf(&out, "gitolite-tui  %s@%s\nTrash\n", m.client.User, m.client.Host)
	out.WriteString(strings.Repeat("─", clamp(m.width, 20, 100)) + "\n")

	listHeight := clamp(m.height-8, 4, 20)
	if m.height == 0 {
		listHeight = 10
	}
	start := 0
	if m.trashCursor >= listHeight {
		start = m.trashCursor - listHeight + 1
	}
	end := min(len(m.trash), start+listHeight)
	for row := start; row < end; row++ {
		marker := "  "
		if row == m.trashCursor {
			marker = "> "
		}
		out.WriteString(marker + truncate(m.trash[row], max(20, m.width-2)) + "\n")
	}
	if len(m.trash) == 0 && !m.loading {
		out.WriteString("  Trash is empty\n")
	}

	m.writePrompt(&out)
	out.WriteByte('\n')
	if m.err != nil {
		fmt.Fprintf(&out, "Error: %v\n", m.err)
	} else {
		out.WriteString(m.status + "\n")
	}
	out.WriteString("↑/↓ select  enter/r restore  R reload  T/esc repositories  q quit\n")
	return out.String()
}

func (m Model) View() string {
	if m.showTrash {
		return m.viewTrash()
	}
	var out strings.Builder
	fmt.Fprintf(&out, "gitolite-tui  %s@%s\n", m.client.User, m.client.Host)
	if m.search {
		fmt.Fprintf(&out, "Search: %s█\n", m.query)
	} else {
		fmt.Fprintf(&out, "Search: %s  (press / to edit)\n", m.query)
	}
	out.WriteString(strings.Repeat("─", clamp(m.width, 20, 100)) + "\n")

	listHeight := clamp(m.height/3, 4, 12)
	if m.height == 0 {
		listHeight = 8
	}
	start := 0
	if m.cursor >= listHeight {
		start = m.cursor - listHeight + 1
	}
	end := min(len(m.filtered), start+listHeight)
	for row := start; row < end; row++ {
		repo := m.repos[m.filtered[row]]
		marker := "  "
		if row == m.cursor {
			marker = "> "
		}
		name := repo.Name
		if repo.Wildcard {
			name += "  [wildcard]"
		}
		fmt.Fprintf(&out, "%s%-6s %s\n", marker, repo.Access, truncate(name, max(10, m.width-11)))
	}
	for row := end - start; row < listHeight; row++ {
		out.WriteByte('\n')
	}

	if repo, ok := m.selected(); ok {
		fmt.Fprintf(&out, "\nClone: %s\n", m.client.CloneURL(repo.Name))
		if repo.Wildcard {
			out.WriteString("This is a wildcard rule; log, refresh, and tig are unavailable.\n")
		}
		if m.active == repo.Name {
			fmt.Fprintf(&out, "Recent commits — %s\n", m.active)
			commitRows := max(1, m.height-listHeight-10)
			if m.height == 0 {
				commitRows = 8
			}
			for i, commit := range m.commits {
				if i >= commitRows {
					break
				}
				line := fmt.Sprintf("%s %s %-14s %s", commit.Hash, commit.Date, commit.Author, commit.Subject)
				out.WriteString(truncate(line, max(20, m.width)) + "\n")
			}
		}
	}

	m.writePrompt(&out)
	out.WriteByte('\n')
	if m.err != nil {
		fmt.Fprintf(&out, "Error: %v\n", m.err)
	} else {
		out.WriteString(m.status + "\n")
	}
	out.WriteString("↑/↓ select  enter log  n new  e desc  d trash  T trash-bin  / search\n")
	out.WriteString("c copy  l clone  r refresh  R reload  t tig  q quit\n")
	return out.String()
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
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

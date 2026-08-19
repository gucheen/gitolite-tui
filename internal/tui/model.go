package tui

import (
	"context"
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

	repos    []gitolite.Repository
	filtered []int
	cursor   int
	query    string
	search   bool
	commits  []repository.Commit
	active   string
	status   string
	err      error
	loading  bool
	width    int
	height   int
}

type reposMsg struct {
	repos []gitolite.Repository
	err   error
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
	return func() tea.Msg {
		repos, err := m.client.List(context.Background())
		return reposMsg{repos: repos, err: err}
	}
}

func (m Model) selected() (gitolite.Repository, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return gitolite.Repository{}, false
	}
	return m.repos[m.filtered[m.cursor]], true
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
			m.status = fmt.Sprintf("Found %d repositories", len(msg.repos))
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
				m.loading, m.err, m.status = true, nil, "Caching and loading "+repo.Name+"…"
				return m, m.loadLogCmd(repo.Name, false)
			}
		case "r":
			if repo, ok := m.selected(); ok {
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
				m.loading, m.err, m.status = true, nil, "Opening tig…"
				return m, m.prepareTigCmd(repo.Name)
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
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
		fmt.Fprintf(&out, "%s%-6s %s\n", marker, repo.Access, truncate(repo.Name, max(10, m.width-11)))
	}
	for row := end - start; row < listHeight; row++ {
		out.WriteByte('\n')
	}

	if repo, ok := m.selected(); ok {
		fmt.Fprintf(&out, "\nClone: %s\n", m.client.CloneURL(repo.Name))
	}
	if m.active != "" {
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

	out.WriteByte('\n')
	if m.err != nil {
		fmt.Fprintf(&out, "Error: %v\n", m.err)
	} else {
		out.WriteString(m.status + "\n")
	}
	out.WriteString("↑/↓ select  enter log  c copy  l clone  r refresh  R reload  t tig  / search  q quit\n")
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

package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gucheng01/gitolite-tui/internal/gitolite"
	"github.com/gucheng01/gitolite-tui/internal/repository"
)

func TestWildcardRuleDoesNotStartRepositoryCommands(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "log", key: tea.KeyMsg{Type: tea.KeyEnter}},
		{name: "refresh", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}},
		{name: "tig", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := New(gitolite.Client{}, repositoryStoreForTest())
			model.repos = []gitolite.Repository{{Name: "CREATOR/..*", Access: "R W", Wildcard: true}}
			model.filtered = []int{0}
			model.active = "previous/repository"

			updated, command := model.Update(test.key)
			if command != nil {
				t.Fatal("wildcard action returned a command")
			}
			got := updated.(Model)
			if !strings.Contains(strings.ToLower(got.status), "wildcard") {
				t.Fatalf("status %q does not explain the wildcard restriction", got.status)
			}
		})
	}
}

func TestTrashRequiresExactRepositoryName(t *testing.T) {
	model := modelWithRepository("team/alice/api", false)

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if command != nil {
		t.Fatal("opening trash confirmation returned a command")
	}
	model = updated.(Model)
	if model.prompt != promptTrash || model.target != "team/alice/api" {
		t.Fatalf("trash prompt = %v target %q", model.prompt, model.target)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("wrong")})
	model = updated.(Model)
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("mismatched confirmation returned a command")
	}
	model = updated.(Model)
	if model.err == nil || model.prompt != promptTrash {
		t.Fatal("mismatched confirmation did not keep the prompt with an error")
	}

	model.input = "team/alice/api"
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("exact confirmation did not return a command")
	}
	model = updated.(Model)
	if model.prompt != promptNone || !model.loading {
		t.Fatal("confirmed trash did not close the prompt and start loading")
	}
}

func TestWildcardRuleCannotBeMovedToTrash(t *testing.T) {
	model := modelWithRepository("CREATOR/..*", true)

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if command != nil {
		t.Fatal("wildcard rule trash returned a command")
	}
	got := updated.(Model)
	if got.prompt != promptNone || !strings.Contains(strings.ToLower(got.status), "wildcard") {
		t.Fatalf("wildcard rule trash status = %q, prompt = %v", got.status, got.prompt)
	}
}

func TestCreateAndDescriptionPrompts(t *testing.T) {
	model := modelWithRepository("team/alice/api", false)

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if command != nil {
		t.Fatal("opening create prompt returned a command")
	}
	model = updated.(Model)
	model.input = "team/alice/new"
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("submitting create prompt did not return a command")
	}

	model = modelWithRepository("team/alice/api", false)
	updated, command = model.Update(descriptionMsg{repo: "team/alice/api", description: "old description"})
	if command != nil {
		t.Fatal("description response returned a command")
	}
	model = updated.(Model)
	if model.prompt != promptDescription || model.input != "old description" {
		t.Fatalf("description prompt = %v input %q", model.prompt, model.input)
	}
	model.input = "new"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("description")})
	model = updated.(Model)
	if model.input != "new description" {
		t.Fatalf("description input after space = %q", model.input)
	}
	_, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("submitting description did not return a command")
	}
}

func TestTrashViewRestoreConfirmation(t *testing.T) {
	model := New(gitolite.Client{}, repositoryStoreForTest())
	model.showTrash = true
	model.trash = []string{"team/alice/api/2026-08-21_10:20:30"}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if command != nil {
		t.Fatal("opening restore confirmation returned a command")
	}
	model = updated.(Model)
	if model.prompt != promptRestore || model.target != model.trash[0] {
		t.Fatalf("restore prompt = %v target %q", model.prompt, model.target)
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("confirming restore did not return a command")
	}
	if updated.(Model).prompt != promptNone {
		t.Fatal("confirming restore did not close the prompt")
	}
}

func TestViewResizesWithTerminal(t *testing.T) {
	for _, trash := range []bool{false, true} {
		t.Run(fmt.Sprintf("trash=%t", trash), func(t *testing.T) {
			model := modelWithManyRepositories(100)
			model.showTrash = trash
			model.cursor, model.trashCursor = 70, 70
			previousRows := 0
			for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 160, Height: 60}, {Width: 80, Height: 24}} {
				updated, _ := model.Update(size)
				model = updated.(Model)
				view := model.View()
				assertViewFits(t, view, size.Width, size.Height)
				if got := len(strings.Split(view, "\n")); got != size.Height {
					t.Fatalf("view has %d rows, want full height %d", got, size.Height)
				}
				if !strings.Contains(view, strings.Repeat("─", size.Width)) {
					t.Fatal("separator does not fill terminal width")
				}
				if !strings.Contains(selectedViewLine(view), "repo-070") {
					t.Fatal("selection is no longer visible after resizing")
				}
				rows := visibleRepositoryRows(view)
				if size.Height == 60 && (rows <= previousRows || rows <= 20) {
					t.Fatalf("large terminal shows %d repositories, previously %d", rows, previousRows)
				}
				if !trash && size.Height == 24 && rows <= 12 {
					t.Fatalf("standard terminal still only shows %d repositories", rows)
				}
				previousRows = rows
			}
		})
	}
}

func TestCommitPreviewReservesSpaceAndCanBeHidden(t *testing.T) {
	model := modelWithManyRepositories(100)
	model.width, model.height = 120, 48
	fullRows := visibleRepositoryRows(model.View())
	model.active = model.repos[0].Name
	for i := 0; i < 100; i++ {
		model.commits = append(model.commits, repository.Commit{Hash: fmt.Sprintf("commit-%03d", i), Subject: "A change"})
	}
	view := model.View()
	assertViewFits(t, view, model.width, model.height)
	previewRows := visibleRepositoryRows(view)
	if previewRows <= 12 || previewRows >= fullRows || !strings.Contains(view, "commit-000") {
		t.Fatalf("preview layout: %d repository rows (without preview: %d)", previewRows, fullRows)
	}
	if strings.Contains(view, "commit-099") || !strings.Contains(view, "q quit") {
		t.Fatal("commits overflowed the available space")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := visibleRepositoryRows(updated.(Model).View()); got != fullRows {
		t.Fatalf("unselected preview still occupies space: %d rows, want %d", got, fullRows)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if got := visibleRepositoryRows(model.View()); got != fullRows || strings.Contains(model.View(), "Recent commits") {
		t.Fatalf("hiding preview did not restore the full list: %d rows, want %d", got, fullRows)
	}
}

func TestViewFitsWithPromptsAndLongContent(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 40, Height: 30}, {Width: 80, Height: 24}, {Width: 160, Height: 60}} {
		for _, prompt := range []promptKind{promptNone, promptCreate, promptDescription, promptTrash, promptRestore} {
			t.Run(fmt.Sprintf("%dx%d/prompt=%d", size.Width, size.Height, prompt), func(t *testing.T) {
				model := modelWithManyRepositories(100)
				model.width, model.height = size.Width, size.Height
				model.repos[0].Name = strings.Repeat("仓库👩‍💻", 40)
				model.repos[0].Wildcard = true
				model.client.Host = strings.Repeat("host", 40)
				model.prompt, model.input, model.target = prompt, strings.Repeat("描述", 40), model.repos[0].Name
				model.query, model.search = strings.Repeat("搜索", 40), true
				model.active = model.repos[0].Name
				model.commits = []repository.Commit{{Hash: "abc123", Subject: strings.Repeat("提交", 80)}}
				model.err = errors.New("operation failed\nwith more details\r\nand\ttabs")
				model.showTrash = prompt == promptRestore
				view := model.View()
				assertViewFits(t, view, size.Width, size.Height)
				if !strings.Contains(view, "Error: operation failed") || !strings.Contains(view, "q quit") {
					t.Fatalf("status or help missing:\n%s", view)
				}
				if prompt != promptNone && !strings.Contains(view, "esc cancel") {
					t.Fatal("prompt controls are not visible")
				}
				if prompt != promptRestore && !strings.HasSuffix(strings.Split(view, "\n")[1], "█") {
					t.Fatal("search cursor is not visible at the end of long input")
				}
				if prompt == promptCreate || prompt == promptDescription || prompt == promptTrash {
					if !strings.Contains(view, "描述█") {
						t.Fatal("prompt does not show the end of long input")
					}
				}
			})
		}
	}
}

func TestViewHandlesEmptyAndTinyTerminals(t *testing.T) {
	for _, trash := range []bool{false, true} {
		model := modelWithManyRepositories(0)
		model.showTrash = trash
		view := model.View()
		assertViewFits(t, view, 80, 24)
		empty := "No repositories found"
		if trash {
			empty = "Trash is empty"
		}
		if !strings.Contains(view, empty) {
			t.Fatalf("empty view missing %q", empty)
		}
		for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 10, Height: 5}, {Width: 80, Height: 3}} {
			updated, _ := model.Update(size)
			assertViewFits(t, updated.(Model).View(), size.Width, size.Height)
		}
	}
}

func TestListPageNavigation(t *testing.T) {
	for _, trash := range []bool{false, true} {
		for _, count := range []int{0, 3, 100} {
			t.Run(fmt.Sprintf("trash=%t/count=%d", trash, count), func(t *testing.T) {
				model := modelWithManyRepositories(count)
				model.width, model.height, model.showTrash = 120, 40, trash
				page := visibleRepositoryRows(model.View())
				for _, test := range []struct {
					key  tea.KeyType
					want int
				}{
					{tea.KeyPgDown, min(max(0, count-1), page)},
					{tea.KeyPgUp, 0},
					{tea.KeyEnd, max(0, count-1)},
					{tea.KeyPgDown, max(0, count-1)},
					{tea.KeyHome, 0},
					{tea.KeyPgUp, 0},
				} {
					updated, command := model.Update(tea.KeyMsg{Type: test.key})
					model = updated.(Model)
					got := model.cursor
					if trash {
						got = model.trashCursor
					}
					if command != nil || got != test.want {
						t.Fatalf("key %v: cursor %d, want %d; command present: %t", test.key, got, test.want, command != nil)
					}
					if count > 0 && !strings.Contains(selectedViewLine(model.View()), fmt.Sprintf("repo-%03d", got)) {
						t.Fatal("navigated selection is not visible")
					}
				}
			})
		}
	}
}

func TestTruncateUsesTerminalCellWidth(t *testing.T) {
	for _, test := range []struct {
		value string
		width int
		want  string
	}{
		{"仓库列表", 5, "仓库…"},
		{"👩‍💻abc", 3, "👩‍💻…"},
		{"e\u0301abc", 3, "e\u0301a…"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
		{"a\nb\tc\r", 10, "a b c "},
	} {
		if got := truncate(test.value, test.width); got != test.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", test.value, test.width, got, test.want)
		}
	}
}

func modelWithManyRepositories(count int) Model {
	model := New(gitolite.Client{User: "git", Host: "example.test"}, repositoryStoreForTest())
	model.loading, model.status = false, "Ready"
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("team/repo-%03d", i)
		model.repos = append(model.repos, gitolite.Repository{Name: name, Access: "R W"})
		model.trash = append(model.trash, name+"/2026-08-28_10:20:30")
	}
	model.applyFilter()
	return model
}

func assertViewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("view has %d rows, terminal has %d", len(lines), height)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("row %d has width %d, terminal has %d: %q", i, got, width, line)
		}
	}
}

func visibleRepositoryRows(view string) int {
	count := 0
	for _, line := range strings.Split(view, "\n") {
		if (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "> ")) && strings.Contains(line, "team/repo-") {
			count++
		}
	}
	return count
}

func selectedViewLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "> ") {
			return line
		}
	}
	return ""
}

func modelWithRepository(name string, wildcard bool) Model {
	model := New(gitolite.Client{}, repositoryStoreForTest())
	model.loading = false
	model.repos = []gitolite.Repository{{Name: name, Access: "R W", Wildcard: wildcard}}
	model.filtered = []int{0}
	return model
}

func repositoryStoreForTest() repository.Store {
	return repository.Store{}
}

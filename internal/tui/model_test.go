package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

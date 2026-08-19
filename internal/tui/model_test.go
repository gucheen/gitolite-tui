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

func repositoryStoreForTest() repository.Store {
	return repository.Store{}
}

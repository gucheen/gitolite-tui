package gitolite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

type Repository struct {
	Name     string
	Access   string
	Wildcard bool
}

type Client struct {
	Host string
	User string
}

func (c Client) target() string {
	return c.User + "@" + c.Host
}

func (c Client) command(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", append([]string{c.target()}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		action := "ssh " + c.target() + " " + strings.Join(args, " ")
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("%s: %w: %s", action, err, message)
		}
		return "", fmt.Errorf("%s: %w", action, err)
	}
	return stdout.String(), nil
}

func (c Client) List(ctx context.Context) ([]Repository, error) {
	output, err := c.command(ctx, "info")
	if err != nil {
		return nil, err
	}
	repos, err := ParseInfo(output)
	if err != nil {
		return nil, fmt.Errorf("parse gitolite info: %w", err)
	}
	return repos, nil
}

func (c Client) Create(ctx context.Context, repo string) error {
	_, err := c.command(ctx, "create", repo)
	return err
}

func (c Client) Description(ctx context.Context, repo string) (string, error) {
	output, err := c.command(ctx, "desc", repo)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c Client) SetDescription(ctx context.Context, repo, description string) error {
	if strings.TrimSpace(description) == "" {
		return errors.New("description cannot be empty")
	}
	_, err := c.command(ctx, "desc", repo, description)
	return err
}

func (c Client) Trash(ctx context.Context, repo string) error {
	_, err := c.command(ctx, "D", "trash", repo)
	return err
}

func (c Client) ListTrash(ctx context.Context) ([]string, error) {
	output, err := c.command(ctx, "D", "list-trash")
	if err != nil {
		return nil, err
	}
	return ParseTrashList(output), nil
}

func (c Client) Restore(ctx context.Context, trashID string) error {
	_, err := c.command(ctx, "D", "restore", trashID)
	return err
}

func (c Client) CloneURL(repo string) string {
	repo = strings.TrimSpace(repo)
	if !strings.HasSuffix(repo, ".git") {
		repo += ".git"
	}
	return fmt.Sprintf("%s:%s", c.target(), repo)
}

var fallbackInfoLine = regexp.MustCompile(`^\s*((?:R|W|C|\+|-)(?:[A-Za-z0-9_+.-]*)(?:\s+(?:R|W|C|\+|-)[A-Za-z0-9_+.-]*)*)\s+([^\s]+)\s*$`)

// ParseInfo parses the access table printed by "ssh git@host info". Header,
// greeting, blank and diagnostic lines are ignored.
func ParseInfo(output string) ([]Repository, error) {
	byName := make(map[string]Repository)
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		var access, name string
		if index := strings.LastIndex(raw, "\t"); index >= 0 {
			access = strings.Join(strings.Fields(raw[:index]), " ")
			name = strings.TrimSpace(raw[index+1:])
		} else if match := fallbackInfoLine.FindStringSubmatch(raw); match != nil {
			access = strings.Join(strings.Fields(match[1]), " ")
			name = strings.TrimSpace(match[2])
		}

		if name == "" || access == "" || !validAccess(access) {
			continue
		}
		byName[name] = Repository{Name: name, Access: access, Wildcard: IsWildcard(name)}
	}

	repos := make([]Repository, 0, len(byName))
	for _, repo := range byName {
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	return repos, nil
}

// IsWildcard reports whether a Gitolite info entry is a repository pattern
// rather than the name of a concrete repository.
func IsWildcard(name string) bool {
	if strings.ContainsAny(name, `*?[](){}^$|\`) {
		return true
	}
	return strings.Contains(name, ".+")
}

func validAccess(access string) bool {
	found := false
	for _, r := range access {
		switch r {
		case 'R', 'W', 'C':
			found = true
		case '+', '-', ' ', '_':
		default:
			return false
		}
	}
	return found
}

func ParseTrashList(output string) []string {
	var entries []string
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if entry := strings.TrimSpace(raw); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

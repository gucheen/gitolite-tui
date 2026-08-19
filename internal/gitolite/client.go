package gitolite

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

type Repository struct {
	Name   string
	Access string
}

type Client struct {
	Host string
	User string
}

func (c Client) target() string {
	return c.User + "@" + c.Host
}

func (c Client) List(ctx context.Context) ([]Repository, error) {
	cmd := exec.CommandContext(ctx, "ssh", c.target(), "info")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("ssh %s info: %w: %s", c.target(), err, message)
		}
		return nil, fmt.Errorf("ssh %s info: %w", c.target(), err)
	}
	repos, err := ParseInfo(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("parse gitolite info: %w", err)
	}
	return repos, nil
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
		byName[name] = Repository{Name: name, Access: access}
	}

	repos := make([]Repository, 0, len(byName))
	for _, repo := range byName {
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	return repos, nil
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

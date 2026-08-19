package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Commit struct {
	Hash    string
	Date    string
	Author  string
	Subject string
}

type Store struct {
	Root     string
	Depth    int
	LogLimit int
}

var unsafePathChar = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s Store) Path(repo string) string {
	base := strings.TrimSuffix(filepath.Base(repo), ".git")
	base = strings.Trim(unsafePathChar.ReplaceAllString(base, "-"), "-.")
	if base == "" {
		base = "repo"
	}
	sum := sha256.Sum256([]byte(repo))
	return filepath.Join(s.Root, base+"-"+hex.EncodeToString(sum[:6])+".git")
}

func (s Store) Exists(repo string) bool {
	info, err := os.Stat(filepath.Join(s.Path(repo), "HEAD"))
	return err == nil && !info.IsDir()
}

func (s Store) Ensure(ctx context.Context, repo, cloneURL string) error {
	if s.Exists(repo) {
		return nil
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	depth := s.Depth
	if depth <= 0 {
		depth = 100
	}
	destination := s.Path(repo)
	temporary, err := os.MkdirTemp(s.Root, ".clone-*")
	if err != nil {
		return fmt.Errorf("create temporary cache: %w", err)
	}
	defer os.RemoveAll(temporary)
	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", "--depth", strconv.Itoa(depth), cloneURL, temporary)
	if output, err := cmd.CombinedOutput(); err != nil {
		return commandError("git clone cache", err, output)
	}
	if err := os.Rename(temporary, destination); err != nil {
		return fmt.Errorf("install repository cache: %w", err)
	}
	return nil
}

func (s Store) Refresh(ctx context.Context, repo string) error {
	if !s.Exists(repo) {
		return errors.New("repository is not cached")
	}
	depth := s.Depth
	if depth <= 0 {
		depth = 100
	}
	cmd := exec.CommandContext(ctx, "git", "--git-dir", s.Path(repo), "fetch", "--prune", "--depth", strconv.Itoa(depth), "origin", "+refs/heads/*:refs/heads/*")
	if output, err := cmd.CombinedOutput(); err != nil {
		return commandError("git fetch cache", err, output)
	}
	return nil
}

func (s Store) Log(ctx context.Context, repo string) ([]Commit, error) {
	refs := exec.CommandContext(ctx, "git", "--git-dir", s.Path(repo), "for-each-ref", "--count=1", "--format=%(refname)")
	refOutput, err := refs.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, commandError("git inspect refs", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("git inspect refs: %w", err)
	}
	if len(bytes.TrimSpace(refOutput)) == 0 {
		return []Commit{}, nil
	}

	limit := s.LogLimit
	if limit <= 0 {
		limit = 30
	}
	format := "%h%x1f%ad%x1f%an%x1f%s%x1e"
	cmd := exec.CommandContext(ctx, "git", "--git-dir", s.Path(repo), "log", "--all", "--date=short", "--pretty=format:"+format, "-n", strconv.Itoa(limit))
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, commandError("git log", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("git log: %w", err)
	}
	return parseLog(output), nil
}

func (s Store) Clone(ctx context.Context, cloneURL, destination string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, destination)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	return nil
}

func (s Store) CloneCommand(cloneURL, destination string) *exec.Cmd {
	return exec.Command("git", "clone", cloneURL, destination)
}

func (s Store) TigCommand(repo string) (*exec.Cmd, error) {
	if !s.Exists(repo) {
		return nil, errors.New("repository is not cached; press enter first")
	}
	if _, err := exec.LookPath("tig"); err != nil {
		return nil, errors.New("tig is not installed or not in PATH")
	}
	return exec.Command("tig", "--git-dir="+s.Path(repo), "--all"), nil
}

func parseLog(output []byte) []Commit {
	records := bytes.Split(output, []byte{0x1e})
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
		record = bytes.TrimSpace(record)
		if len(record) == 0 {
			continue
		}
		fields := bytes.SplitN(record, []byte{0x1f}, 4)
		if len(fields) != 4 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    string(fields[0]),
			Date:    string(fields[1]),
			Author:  string(fields[2]),
			Subject: string(fields[3]),
		})
	}
	return commits
}

func commandError(action string, err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}

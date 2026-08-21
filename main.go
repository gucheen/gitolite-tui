package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gucheng01/gitolite-tui/internal/config"
	"github.com/gucheng01/gitolite-tui/internal/gitolite"
	"github.com/gucheng01/gitolite-tui/internal/repository"
	appui "github.com/gucheng01/gitolite-tui/internal/tui"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gitolite-tui:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("gitolite-tui", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	host := flags.String("host", "", "Gitolite SSH host (saved to XDG config when provided)")
	user := flags.String("user", "", "Gitolite SSH user (default: git)")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = usage
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if parsed := flags.Args(); len(parsed) > 0 && (parsed[0] == "help" || parsed[0] == "-h" || parsed[0] == "--help") {
		usage()
		return nil
	}

	changed := false
	if *host != "" {
		cfg.Host = *host
		changed = true
	}
	if *user != "" {
		cfg.User = *user
		changed = true
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if changed {
		if err := config.Save(cfg); err != nil {
			return err
		}
	}

	root, err := config.CacheRoot()
	if err != nil {
		return err
	}
	client := gitolite.Client{Host: cfg.Host, User: cfg.User}
	store := repository.Store{Root: root, Depth: 100, LogLimit: cfg.LogLimit}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := "tui"
	rest := flags.Args()
	if len(rest) > 0 {
		command, rest = rest[0], rest[1:]
	}
	switch command {
	case "list":
		if len(rest) != 0 {
			return errors.New("usage: gitolite-tui list")
		}
		repos, err := client.List(ctx)
		if err != nil {
			return err
		}
		for _, repo := range repos {
			fmt.Printf("%-7s\t%s\t%s\n", repo.Access, repo.Name, client.CloneURL(repo.Name))
		}
		return nil
	case "url":
		if len(rest) != 1 {
			return errors.New("usage: gitolite-tui url <repo>")
		}
		fmt.Println(client.CloneURL(rest[0]))
		return nil
	case "log":
		if len(rest) != 1 {
			return errors.New("usage: gitolite-tui log <repo>")
		}
		repo := rest[0]
		if gitolite.IsWildcard(repo) {
			return fmt.Errorf("%q is a wildcard repository rule; logs are only available for concrete repositories", repo)
		}
		if err := store.Ensure(ctx, repo, client.CloneURL(repo)); err != nil {
			return err
		}
		commits, err := store.Log(ctx, repo)
		if err != nil {
			return err
		}
		for _, commit := range commits {
			fmt.Printf("%s\t%s\t%s\t%s\n", commit.Hash, commit.Date, commit.Author, commit.Subject)
		}
		return nil
	case "clone":
		if len(rest) < 1 || len(rest) > 2 {
			return errors.New("usage: gitolite-tui clone <repo> [directory]")
		}
		destination := ""
		if len(rest) == 2 {
			destination = rest[1]
		} else {
			destination = strings.TrimSuffix(filepath.Base(rest[0]), ".git")
			if destination == "" || destination == "." {
				return fmt.Errorf("cannot derive clone directory from %q", rest[0])
			}
		}
		return store.Clone(ctx, client.CloneURL(rest[0]), destination)
	case "create":
		if len(rest) != 1 {
			return errors.New("usage: gitolite-tui create <repo>")
		}
		return client.Create(ctx, rest[0])
	case "desc":
		if len(rest) == 0 {
			return errors.New("usage: gitolite-tui desc <repo> [description]")
		}
		if len(rest) == 1 {
			description, err := client.Description(ctx, rest[0])
			if err != nil {
				return err
			}
			fmt.Println(description)
			return nil
		}
		return client.SetDescription(ctx, rest[0], strings.Join(rest[1:], " "))
	case "trash":
		if len(rest) != 1 {
			return errors.New("usage: gitolite-tui trash <repo>")
		}
		return client.Trash(ctx, rest[0])
	case "trash-list":
		if len(rest) != 0 {
			return errors.New("usage: gitolite-tui trash-list")
		}
		entries, err := client.ListTrash(ctx)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			fmt.Println(entry)
		}
		return nil
	case "restore":
		if len(rest) != 1 {
			return errors.New("usage: gitolite-tui restore <trash-id>")
		}
		return client.Restore(ctx, rest[0])
	case "tui":
		if len(rest) != 0 {
			return errors.New("usage: gitolite-tui tui")
		}
		program := tea.NewProgram(appui.New(client, store), tea.WithAltScreen())
		_, err := program.Run()
		return err
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func validateConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		path, _ := config.Path()
		return fmt.Errorf("no Gitolite host configured; run gitolite-tui --host HOST list (config: %s)", path)
	}
	if strings.HasPrefix(cfg.Host, "-") || strings.ContainsAny(cfg.Host, " \t\r\n@") {
		return fmt.Errorf("invalid host %q", cfg.Host)
	}
	if cfg.User == "" || strings.HasPrefix(cfg.User, "-") || strings.ContainsAny(cfg.User, " \t\r\n@:") {
		return fmt.Errorf("invalid SSH user %q", cfg.User)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `gitolite-tui - browse and clone Gitolite repositories

Usage:
  gitolite-tui [--host HOST] [--user USER] list
  gitolite-tui [--host HOST] [--user USER] url <repo>
  gitolite-tui [--host HOST] [--user USER] log <repo>
  gitolite-tui [--host HOST] [--user USER] clone <repo> [directory]
  gitolite-tui [--host HOST] [--user USER] create <repo>
  gitolite-tui [--host HOST] [--user USER] desc <repo> [description]
  gitolite-tui [--host HOST] [--user USER] trash <repo>
  gitolite-tui [--host HOST] [--user USER] trash-list
  gitolite-tui [--host HOST] [--user USER] restore <trash-id>
  gitolite-tui [--host HOST] [--user USER] tui

Environment:
  GITOLITE_HOST and GITOLITE_USER override values loaded from XDG config.`)
}

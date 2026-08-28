# gitolite-tui

[简体中文](README.zh-CN.md)

`gitolite-tui` is a Gitolite repository browser built with Go and Bubble Tea.
It discovers repositories available to the current SSH user by running
`ssh git@HOST info`, maintains shallow bare clones in the XDG cache directory,
and provides both an interactive terminal UI and script-friendly commands.

## Features

- Browse and search accessible Gitolite repositories.
- Automatically fit the terminal size, showing more repositories in taller windows.
- Display wildcard repository rules without attempting to load logs or open
  them with `tig`.
- View the SSH clone URL for each repository.
- Cache shallow bare clones and inspect recent commits.
- Copy clone URLs, clone repositories locally, refresh cached data, and open
  repositories with `tig`.
- Use the same functionality through non-interactive commands.
- Execute `ssh` and `git` directly with argument arrays, without shell command
  construction.

## Installation and configuration

```sh
go install .
gitolite-tui --host git.example.com list
```

The first use of `--host`, and optionally `--user`, saves the settings to:

```text
$XDG_CONFIG_HOME/gitolite-tui/config.json
```

If `XDG_CONFIG_HOME` is not set, the platform's standard user configuration
directory is used. `GITOLITE_HOST` and `GITOLITE_USER` can temporarily override
the saved values.

The SSH user defaults to `git`:

```sh
gitolite-tui --host git.example.com --user git list
```

## Commands

```text
gitolite-tui list
gitolite-tui url <repo>
gitolite-tui log <repo>
gitolite-tui clone <repo> [directory]
gitolite-tui create <repo>
gitolite-tui desc <repo> [description]
gitolite-tui trash <repo>
gitolite-tui trash-list
gitolite-tui restore <trash-id>
gitolite-tui tui
```

Running `gitolite-tui` without a command also starts the TUI.

## TUI key bindings

| Key | Action |
| --- | --- |
| `/` | Edit the repository search query |
| `Up` / `Down`, `j` / `k` | Select a repository |
| `Page Up` / `Page Down` | Move by one visible page (also in trash) |
| `Home` / `End` | Jump to the first / last repository (also in trash) |
| `Enter` | Cache the selected repository and show recent commits |
| `Esc` | Hide the commit preview and expand the repository list |
| `n` | Create a wildcard repository |
| `e` | Edit the selected repository description |
| `d` | Move the selected repository to trash |
| `T` | Open the trash and restore repositories |
| `c` | Copy the clone URL |
| `l` | Clone the repository into the current directory |
| `r` | Refresh the selected cached repository |
| `R` | Reload the repository list from Gitolite |
| `t` | Open the cached repository with `tig` |
| `q` | Quit |

The layout updates when the terminal is resized. The repository list uses the
available height, sharing space with recent commits only while their repository
is selected. Long lines are truncated to the terminal width.

The create, description, and trash features require the corresponding `create`,
`desc`, and `D` remote commands to be enabled in the Gitolite server's
`.gitolite.rc`. Deletion only uses recoverable `D trash`; the application never
runs permanent `D rm` deletion.

## Cache

Repository caches are stored under:

```text
$XDG_CACHE_HOME/gitolite-tui/repos
```

If `XDG_CACHE_HOME` is not set, the platform's standard user cache directory is
used. Each repository is stored as a shallow bare clone.

## Development

```sh
go test ./...
go vet ./...
go build ./...
```

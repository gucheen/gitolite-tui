# gitolite-tui

[English](README.md)

一个用 Go 和 Bubble Tea 编写的 Gitolite 仓库浏览器。它通过
`ssh git@HOST info` 获取可访问仓库，在 XDG 缓存目录维护浅 bare clone，
并同时提供 TUI 和可脚本化的子命令。

## 安装和配置

```sh
go install .
gitolite-tui --host git.example.com list
```

首次使用 `--host`（以及可选的 `--user`）时会把设置写入
`$XDG_CONFIG_HOME/gitolite-tui/config.json`。也可以使用环境变量
`GITOLITE_HOST` 和 `GITOLITE_USER` 临时覆盖。

## 命令

```text
gitolite-tui list
gitolite-tui url <repo>
gitolite-tui log <repo>
gitolite-tui clone <repo> [directory]
gitolite-tui tui
```

TUI 按键：`/` 搜索，方向键或 `j/k` 选择，`enter` 缓存并显示提交，
`c` 复制 Clone 地址，`l` Clone 到当前目录，`r` 刷新当前仓库，`R` 重新
获取仓库列表，`t` 用 tig 打开缓存，`q` 退出。

缓存位于 `$XDG_CACHE_HOME/gitolite-tui/repos`。所有 `ssh` 和 `git` 调用
均通过参数数组执行，不经过 Shell。

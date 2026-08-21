# gitolite-tui

[English](README.md)

一个用 Go 和 Bubble Tea 编写的 Gitolite 仓库浏览器。它通过
`ssh git@HOST info` 获取可访问仓库，在 XDG 缓存目录维护浅 bare clone，
并同时提供 TUI 和可脚本化的子命令。

Wildcard 仓库规则会保留在列表中，但不会加载日志，也不能用 `tig` 打开。

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
gitolite-tui create <repo>
gitolite-tui desc <repo> [description]
gitolite-tui trash <repo>
gitolite-tui trash-list
gitolite-tui restore <trash-id>
gitolite-tui tui
```

TUI 按键：`/` 搜索，方向键或 `j/k` 选择，`enter` 缓存并显示提交，
`n` 创建 wildcard 仓库，`e` 编辑描述，`d` 将仓库移入 Trash，`T` 查看
Trash 并恢复仓库，`c` 复制 Clone 地址，`l` Clone 到当前目录，`r` 刷新
当前仓库，`R` 重新获取仓库列表，`t` 用 tig 打开缓存，`q` 退出。

创建、描述和 Trash 功能需要 Gitolite 服务器在 `.gitolite.rc` 中启用对应的
`create`、`desc` 和 `D` 远程命令。删除功能只调用可恢复的 `D trash`，不会
执行 `D rm` 永久删除。

缓存位于 `$XDG_CACHE_HOME/gitolite-tui/repos`。所有 `ssh` 和 `git` 调用
均通过参数数组执行，不经过 Shell。

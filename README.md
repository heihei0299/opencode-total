# OpenCode Analyzer

独立的 OpenCode 云端用量同步、归档与对账工具，使用 Go 实现，不依赖 token-analyzer。数据保存在独立目录，可导出 JSON / CSV，并提供本地 HTTP API 与 Web 面板。

## 运行

需要 Go 1.23+。配置 OpenCode 凭据、工作区 ID 和数据目录：

~~~sh
export OPENCODE_AUTH='...'
export OPENCODE_WORKSPACE_ID='wrk_...'
export OPENCODE_DATA_DIR="$HOME/.local/share/opencode-analyzer"
go run ./cmd/opencode-analyzer sync
go run ./cmd/opencode-analyzer export --format json
go run ./cmd/opencode-analyzer serve --pi-dir ~/.pi/agent/sessions
~~~

也可在当前目录或数据目录使用 `.env`。凭据优先级为 CLI 参数 > 环境变量 > `.env`。不要提交凭据、数据库或真实导出文件。

## CLI 与 API

~~~text
opencode-analyzer sync [--auth <cookie>] [--workspace <id>] [--data-dir <dir>] [--full] [--limit <pages>]
opencode-analyzer export [--format json|csv] [--output <file>] [--month <YYYY-MM>]
opencode-analyzer serve [--host <host>] [--port <port>] [--data-dir <dir>] [--pi-dir <dir>]
~~~

`sync` 更新当前月份成本并同步使用历史；`--full` 忽略增量游标，`--limit` 限制抓取页数。`serve` 默认监听 `127.0.0.1:50800`，访问 `/` 打开面板。

API 路由：`GET /api/opencode/costs`、`GET /api/opencode/history`、`GET /api/opencode/audit`、`POST /api/opencode/sync`。

本地 Pi 对账只读取 `--pi-dir` 指定的会话 JSONL。测试使用合成数据和 mock 文件，不访问 OpenCode 网络服务。
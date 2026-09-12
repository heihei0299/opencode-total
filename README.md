# OpenCode Analyzer

独立的 OpenCode 云端用量同步、归档与对账工具。该目录可以整体迁移到单独仓库；它不导入或依赖 `token-analyzer` 的任何 `internal/*`、数据库、HTTP server 或 WebUI。

## 功能

- 通过 OpenCode SolidStart `/_server` RPC 获取工作区、月度成本和分页使用历史；
- Seroval envelope / SolidStart `$R` 响应解码；
- 将数据保存到独立 `data/opencode/`：`costs.json`、`history.json`、`history.csv`；
- 按记录 ID 与 `lastSyncedTime` 增量同步，跨进程文件锁避免并发写入；
- `sync` / `export` CLI；
- 独立 HTTP API 与 WebUI，包含月度模型成本图、历史明细和本地 Pi 对账；本地对账自带 assistant/toolResult/compaction/branch_summary 四载体、billable/cost/failed 门控、fork/request/semantic 去重与月份边界处理；
- 本地 Pi 对账只读取 `--pi-dir` 指定的 session JSONL，不依赖 token-analyzer。

## 运行

需要 Go 1.23+：

```bash
export OPENCODE_AUTH='...'
export OPENCODE_WORKSPACE_ID='wrk_...'
export OPENCODE_DATA_DIR="$HOME/.local/share/opencode-analyzer"
go run ./cmd/opencode-analyzer sync
go run ./cmd/opencode-analyzer export --format json
go run ./cmd/opencode-analyzer serve --pi-dir ~/.pi/agent/sessions
```

也可以在临时切换账号时使用 `sync --auth <cookie> --workspace <id>`，但环境变量或 `.env` 可避免凭据进入 shell history。

凭证优先级为 CLI 参数 > 进程环境变量 > 当前目录/数据目录 `.env`。OpenCode cookie 只由本项目读取；不要把 `.env`、数据库或真实导出文件提交到 Git。

### CLI

```text
opencode-analyzer sync [--auth <cookie>] [--workspace <id>] [--data-dir <dir>] [--pi-dir <dir>] [--full] [--limit <pages>]
opencode-analyzer export [--format json|csv] [--output <file>] [--month <YYYY-MM>] [--data-dir <dir>]
opencode-analyzer serve [--host <host>] [--port <port>] [--data-dir <dir>] [--pi-dir <dir>]
```

`sync` 会保存当前月份成本并从第 0 页开始同步使用历史。`--full` 忽略增量游标，`--limit` 限制最多抓取页数。

## HTTP API

`serve` 默认监听 `127.0.0.1:50800`：

- `GET /api/opencode/costs?year=YYYY&month=M`
- `GET /api/opencode/history?page=N&size=M&model=X&session=Y`
- `GET /api/opencode/audit?year=YYYY&month=M`
- `POST /api/opencode/sync`，JSON body 可选 `auth`、`workspaceId` / `workspace`

访问 `/` 即可打开独立 OpenCode 对账面板。API 不提供 token-analyzer 的 Pi/Codex 查询路由。

## 测试

```bash
go test ./...
```

测试只使用合成数据和 mock 文件，不访问 OpenCode 网络服务。

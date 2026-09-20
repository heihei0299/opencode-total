package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
	"github.com/heihei0299/opencode-analyzer/internal/server"
	"github.com/heihei0299/opencode-analyzer/internal/syncer"
)

const version = "2026.9.3"

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printHelp()
		return
	}
	if args[0] == "-v" || args[0] == "--version" {
		fmt.Printf("opencode-analyzer %s\n", version)
		return
	}

	var err error
	switch args[0] {
	case "sync":
		err = runSync(args[1:])
	case "export":
		err = runExport(args[1:])
	case "serve":
		err = runServe(args[1:])
	default:
		err = fmt.Errorf("未知子命令: %s（支持 sync / export / serve）", args[0])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`用法:
  opencode-analyzer sync [选项]
  opencode-analyzer export [选项]
  opencode-analyzer serve [选项]

sync 选项:
  --auth <cookie>       OpenCode auth（也可用 OPENCODE_AUTH）
  --workspace <id>      工作区 ID（也可用 OPENCODE_WORKSPACE_ID）
  --data-dir <dir>      本地数据目录（默认 data/opencode）
  --full                忽略增量游标，全量同步
  --limit <pages>       最多抓取页数

export 选项:
  --format json|csv     导出格式（默认 json）
  --output <file>       写入文件；缺省输出到 stdout
  --month <YYYY-MM>     仅导出指定月份
  --data-dir <dir>     本地数据目录

serve 选项:
  --host <host>         监听地址（默认 127.0.0.1）
  --port <port>         监听端口（默认 50800）
  --data-dir <dir>     本地数据目录
  --pi-dir <dir>       本地 Pi session 目录（仅供审计）`)
}

func runSync(args []string) error {
	return runSyncWithRunner(args, syncer.Run)
}

func runSyncWithRunner(args []string, runner func(syncer.Options) (opencode.SyncResult, error)) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	auth := fs.String("auth", "", "OpenCode auth")
	workspace := fs.String("workspace", "", "OpenCode workspace")
	dataDir := fs.String("data-dir", "", "local data directory")
	full := fs.Bool("full", false, "full sync")
	limit := fs.Int("limit", 0, "page limit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit < 0 {
		return fmt.Errorf("无效 limit: %d（需为非负整数）", *limit)
	}
	result, err := runner(syncer.Options{
		Auth:      *auth,
		Workspace: *workspace,
		DataDir:   *dataDir,
		Full:      *full,
		Limit:     *limit,
	})
	if err != nil {
		if result.Status == opencode.SyncFailed {
			fmt.Printf("同步失败: 状态 %s, 原因 %s, 抓取 %d 页, lastSyncedTime: %s, %v\n", result.Status, result.Reason, result.Pages, result.LastSyncedTime, err)
		}
		return err
	}
	fmt.Printf("同步完成: 抓取 %d 页, 新增 %d 条, 更新 %d 条, 状态 %s, 耗时 %dms\nlastSyncedTime: %s\n", result.Pages, result.Added, result.Updated, result.Status, result.ElapsedMs, result.LastSyncedTime)
	for _, warning := range result.Warnings {
		fmt.Printf("warning: %s\n", warning)
	}
	if result.Status == opencode.SyncPartial {
		return fmt.Errorf("同步未完整完成")
	}
	return nil
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	format := fs.String("format", "json", "json or csv")
	output := fs.String("output", "", "output path")
	month := fs.String("month", "", "YYYY-MM")
	dataDir := fs.String("data-dir", "", "local data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "json" && *format != "csv" {
		return fmt.Errorf("未知格式: %s（支持 json/csv）", *format)
	}

	storage := opencode.NewStorage(opencode.ResolveDataDir(*dataDir, os.Getenv("OPENCODE_DATA_DIR")))
	history, err := storage.LoadHistory()
	if err != nil {
		return fmt.Errorf("读取历史失败: %w", err)
	}
	records := history.Records
	if *month != "" {
		yearNumber, monthNumber, ok := opencode.ParseMonth(*month)
		if !ok {
			return fmt.Errorf("无效月份: %s（需为 YYYY-MM）", *month)
		}
		records = opencode.RecordsInMonth(records, yearNumber, monthNumber, time.Local)
	}
	var content []byte
	if *format == "json" {
		content, err = json.MarshalIndent(records, "", "  ")
		if err == nil {
			content = append(content, '\n')
		}
	} else {
		content, err = opencode.EncodeCSV(records)
	}
	if err != nil {
		return fmt.Errorf("导出失败: %w", err)
	}
	if *output == "" {
		_, err = os.Stdout.Write(content)
		return err
	}
	if dir := filepath.Dir(*output); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(*output, content, 0644); err != nil {
		return err
	}
	fmt.Printf("导出完成: %d 条记录 → %s\n", len(records), *output)
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", "127.0.0.1", "listen host")
	port := fs.Int("port", 50800, "listen port")
	dataDir := fs.String("data-dir", "", "local data directory")
	piDir := fs.String("pi-dir", defaultPiDir(), "Pi session directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("无效端口: %d（需为 0-65535 的整数）", *port)
	}
	resolvedDataDir := opencode.ResolveDataDir(*dataDir, os.Getenv("OPENCODE_DATA_DIR"))
	srv := server.NewServer(*piDir, server.Options{DataDir: resolvedDataDir})
	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("OpenCode Analyzer WebUI: http://%s/\n数据目录: %s\n", addr, resolvedDataDir)
	return srv.ListenAndServe(addr)
}

func defaultPiDir() string {
	if value := strings.TrimSpace(os.Getenv("PI_SESSION_DIR")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

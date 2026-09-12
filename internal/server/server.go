package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
	"github.com/heihei0299/opencode-analyzer/internal/piaudit"
	"github.com/heihei0299/opencode-analyzer/internal/syncer"
)

//go:embed webui.html
var webUI embed.FS

type Options struct {
	DataDir string
}

type Server struct {
	piDir string
	mux   *http.ServeMux
	store *opencode.Storage
}

func NewServer(piDir string, options ...Options) *Server {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if strings.TrimSpace(piDir) == "" {
		piDir = strings.TrimSpace(os.Getenv("PI_SESSION_DIR"))
	}
	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("OPENCODE_DATA_DIR"))
	}
	if dataDir == "" {
		dataDir = "data/opencode"
	}
	s := &Server{piDir: piDir, mux: http.NewServeMux(), store: opencode.NewStorage(dataDir)}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) ListenAndServe(addr string) error {
	if err := validateLoopbackAddr(addr); err != nil {
		return err
	}
	return http.ListenAndServe(addr, s.mux)
}

func validateLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("无效监听地址: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("仅支持 loopback 监听地址: %s", host)
	}
	return nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/api/opencode/costs", s.handleCosts)
	s.mux.HandleFunc("/api/opencode/history", s.handleHistory)
	s.mux.HandleFunc("/api/opencode/audit", s.handleAudit)
	s.mux.HandleFunc("/api/opencode/sync", s.handleSync)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		sendError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("路径不存在: %s", r.URL.Path))
		return
	}
	content, err := webUI.ReadFile("webui.html")
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusNotFound, "Not Found", "该路径只支持 GET")
		return
	}
	year, month, err := parseYearMonth(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if err := s.store.EnsureDataDir(); err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	costs, err := s.store.GetCosts(year, month)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	if costs == nil {
		costs = &opencode.CostsResult{Usage: []opencode.MonthlyCostItem{}, Keys: []opencode.KeyInfo{}}
	}
	sendJSON(w, http.StatusOK, map[string]any{"year": year, "month": month, "costs": costs})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusNotFound, "Not Found", "该路径只支持 GET")
		return
	}
	page, size, err := parsePaging(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if err := s.store.EnsureDataDir(); err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	filter := opencode.HistoryFilter{Model: r.URL.Query().Get("model")}
	filter.SessionID = r.URL.Query().Get("session")
	if filter.SessionID == "" {
		filter.SessionID = r.URL.Query().Get("sessionID")
	}
	if filter.SessionID == "" {
		filter.SessionID = r.URL.Query().Get("sessionId")
	}
	rows, total, err := s.store.GetHistory(filter, page, size)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	out := map[string]any{"rows": rows, "total": total}
	if page > 0 {
		out["page"] = page
		out["size"] = size
	}
	sendJSON(w, http.StatusOK, out)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusNotFound, "Not Found", "该路径只支持 GET")
		return
	}
	year, month, err := parseYearMonth(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	local, err := piaudit.TotalsForMonth(s.piDir, year, month)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	history, err := s.store.LoadHistory()
	if err != nil {
		sendError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	audit := opencode.BuildAudit(local, piaudit.RecordsInMonth(history.Records, year, month), year, month)
	sendJSON(w, http.StatusOK, audit)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusNotFound, "Not Found", "该路径只支持 POST")
		return
	}
	var body struct {
		Auth        string `json:"auth"`
		WorkspaceID string `json:"workspaceId"`
		Workspace   string `json:"workspace"`
		Full        bool   `json:"full"`
		Limit       int    `json:"limit"`
	}
	data, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if len(data) > 0 && json.Unmarshal(data, &body) != nil {
		sendError(w, http.StatusBadRequest, "Bad Request", "请求体不是合法 JSON")
		return
	}
	if body.Limit < 0 {
		sendError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("无效 limit: %d（需为非负整数）", body.Limit))
		return
	}
	workspace := body.WorkspaceID
	if workspace == "" {
		workspace = body.Workspace
	}
	result, err := syncer.Run(syncer.Options{
		Auth:      body.Auth,
		Workspace: workspace,
		DataDir:   s.store.DataDir(),
		Full:      body.Full,
		Limit:     body.Limit,
	})
	if err != nil {
		reason := result.Reason
		if reason == "" {
			reason = opencode.SyncErrorReasonOf(err)
		}
		payload := map[string]any{
			"added":          result.Added,
			"updated":        result.Updated,
			"pages":          result.Pages,
			"elapsedMs":      result.ElapsedMs,
			"lastSyncedTime": result.LastSyncedTime,
			"status":         opencode.SyncFailed,
			"reason":         reason,
			"detail":         err.Error(),
		}
		if len(result.Warnings) > 0 {
			payload["warnings"] = result.Warnings
		}
		if reason == opencode.SyncReasonConflict {
			payload["error"] = "Conflict"
			sendJSON(w, http.StatusConflict, payload)
		} else {
			payload["error"] = "Internal Server Error"
			sendJSON(w, http.StatusInternalServerError, payload)
		}
		return
	}
	sendJSON(w, http.StatusOK, result)
}

func parseYearMonth(r *http.Request) (int, int, error) {
	yearRaw := r.URL.Query().Get("year")
	monthRaw := r.URL.Query().Get("month")
	if len(yearRaw) != 4 {
		return 0, 0, fmt.Errorf("缺少或无效 year 参数（需为 4 位年份）")
	}
	year, err := strconv.Atoi(yearRaw)
	if err != nil || year < 1000 || year > 9999 {
		return 0, 0, fmt.Errorf("无效 year: %s（需为 4 位年份）", yearRaw)
	}
	month, err := strconv.Atoi(monthRaw)
	if err != nil || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("无效 month: %s（需为 1-12 的整数）", monthRaw)
	}
	return year, month, nil
}

func parsePaging(r *http.Request) (int, int, error) {
	pageRaw, pageOK := r.URL.Query()["page"]
	sizeRaw, sizeOK := r.URL.Query()["size"]
	if !pageOK && !sizeOK {
		return 0, 0, nil
	}
	if !pageOK || !sizeOK {
		return 0, 0, fmt.Errorf("page 与 size 必须同时提供")
	}
	page, err := strconv.Atoi(pageRaw[0])
	if err != nil || page < 1 || page > 200 {
		return 0, 0, fmt.Errorf("无效 page: %s（需为 1-200 的整数）", pageRaw[0])
	}
	size, err := strconv.Atoi(sizeRaw[0])
	if err != nil || size < 1 || size > 200 {
		return 0, 0, fmt.Errorf("无效 size: %s（需为 1-200 的整数）", sizeRaw[0])
	}
	return page, size, nil
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, name, detail string) {
	sendJSON(w, status, map[string]string{"error": name, "detail": detail})
}

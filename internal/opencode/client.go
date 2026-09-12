package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	RPCURL = "https://opencode.ai/_server"

	FnWorkspaces   = "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f"
	FnMonthlyCosts = "15702f3a12ff8bff357f8c2aa154a17e65b746d5f6b96adc9002c86ee0c15205"
	FnUsageHistory = "bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c"
)

type Client struct {
	auth       string
	httpClient *http.Client
}

func NewClient(auth string) *Client {
	return NewClientWithHTTPClient(auth, &http.Client{Timeout: 15 * time.Second})
}

func NewClientWithHTTPClient(auth string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{auth: strings.TrimSpace(auth), httpClient: httpClient}
}

func retryableRPCError(err error) bool {
	reason := SyncErrorReasonOf(err)
	return reason == SyncReasonNetwork || reason == SyncReasonServer
}

func wrapRPCError(err error) error {
	if err == nil || SyncErrorReasonOf(err) != SyncReasonInternal {
		return err
	}
	message := err.Error()
	switch {
	case strings.HasPrefix(message, "网络超时:"):
		return NewSyncError(SyncReasonNetwork, err)
	case strings.HasPrefix(message, "认证失效:"):
		return NewSyncError(SyncReasonAuthentication, err)
	case strings.Contains(message, "HTTP 404"):
		return NewSyncError(SyncReasonNotFound, err)
	case strings.HasPrefix(message, "服务器错误:"):
		return NewSyncError(SyncReasonServer, err)
	case strings.HasPrefix(message, "请求失败:"):
		return NewSyncError(SyncReasonHTTP, err)
	default:
		return NewSyncError(SyncReasonDecode, err)
	}
}

func (c *Client) buildCookieHeader() string {
	raw := c.auth
	var cookieBase string
	if strings.HasPrefix(raw, "auth=") || strings.Contains(raw, "auth=") {
		cookieBase = raw
	} else {
		cookieBase = "auth=" + raw
	}

	ocLocale := os.Getenv("oc_locale")
	if ocLocale == "" {
		ocLocale = os.Getenv("OC_LOCALE")
	}
	if ocLocale != "" && !strings.Contains(cookieBase, "oc_locale") {
		return cookieBase + "; oc_locale=" + ocLocale
	}
	return cookieBase
}

func (c *Client) rpc(fnID string, args []any) (value any, err error) {
	defer func() { err = wrapRPCError(err) }()
	if c.auth == "" {
		return nil, fmt.Errorf("认证失效: 缺少 OpenCode auth，请设置 OPENCODE_AUTH 或传入 --auth（凭证过期/缺失）")
	}

	cookieHeader := c.buildCookieHeader()

	// 对 usageHistory 和 monthlyCosts 使用 GET 模式
	isUsageOrCosts := (fnID == FnUsageHistory || fnID == FnMonthlyCosts)
	if isUsageOrCosts {
		argsBytes, _ := json.Marshal(args)
		getURL := fmt.Sprintf("%s?id=%s&args=%s", RPCURL, url.QueryEscape(fnID), url.QueryEscape(string(argsBytes)))

		req, err := http.NewRequest("GET", getURL, nil)
		if err != nil {
			return nil, err
		}

		instanceVal := "server-fn:1"
		if fnID == FnMonthlyCosts {
			instanceVal = "server-fn:0"
		}
		var workspaceID string
		if len(args) > 0 {
			workspaceID = fmt.Sprintf("%v", args[0])
		}

		req.Header.Set("Cookie", cookieHeader)
		req.Header.Set("X-Server-Id", fnID)
		req.Header.Set("X-Server-Instance", instanceVal)
		req.Header.Set("Referer", fmt.Sprintf("https://opencode.ai/workspace/%s/usage", workspaceID))
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("网络超时: 请求 OpenCode 超时，请检查网络 — %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, fmt.Errorf("认证失效: OpenCode 凭证已过期或无效（HTTP %d），请刷新 auth cookie（凭证过期）", resp.StatusCode)
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("请求失败: OpenCode 返回 HTTP 404（Function ID 可能已随前端发版更换）")
		}
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("服务器错误: OpenCode 服务异常（HTTP %d），请稍后重试", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("请求失败: OpenCode 返回 HTTP %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("网络超时: 读取 OpenCode 响应失败 — %v", err)
		}
		return DecodeResponseText(string(bodyBytes))
	}

	// POST 模式
	payload := EncodePayload(args)
	req, err := http.NewRequest("POST", RPCURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("X-Server-Id", fnID)
	req.Header.Set("X-Server-Instance", "server-fn:0")
	req.Header.Set("Referer", "https://opencode.ai/")
	req.Header.Set("Origin", "https://opencode.ai")
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络超时: 请求 OpenCode 超时，请检查网络 — %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("认证失效: OpenCode 凭证已过期或无效（HTTP %d），请刷新 auth cookie（凭证过期）", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("请求失败: OpenCode 返回 HTTP 404（Function ID 可能已随前端发版更换）")
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, fmt.Errorf("服务器错误: OpenCode 服务异常（HTTP %d），请稍后重试", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败: OpenCode 返回 HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("网络超时: 读取 OpenCode 响应失败 — %v", err)
	}
	return DecodeResponseText(string(bodyBytes))
}

func (c *Client) GetWorkspaces() ([]WorkspaceInfo, error) {
	raw, err := c.rpc(FnWorkspaces, []any{})
	if err != nil {
		return nil, err
	}
	workspaces, err := decodeWorkspaceList(raw)
	if err != nil {
		return nil, NewSyncError(SyncReasonDecode, err)
	}
	return workspaces, nil
}

func decodeWorkspaceList(raw any) ([]WorkspaceInfo, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "null" {
		return nil, fmt.Errorf("workspace 响应不能为 null")
	}
	if strings.HasPrefix(text, "[") {
		var list []WorkspaceInfo
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
		return list, nil
	}

	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	for _, key := range []string{"workspaces", "data"} {
		value, ok := wrapper[key]
		if !ok || strings.TrimSpace(string(value)) == "null" {
			continue
		}
		var list []WorkspaceInfo
		if err := json.Unmarshal(value, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	return nil, fmt.Errorf("无法识别 OpenCode workspace 响应")
}

func (c *Client) GetMonthlyCosts(workspaceID string, yearMonth ...int) (*CostsResult, error) {
	args := []any{workspaceID}
	if len(yearMonth) >= 2 {
		args = append(args, yearMonth[0], yearMonth[1])
	}
	raw, err := c.rpc(FnMonthlyCosts, args)
	if err != nil {
		return nil, err
	}
	result, err := decodeCostsResult(raw)
	if err != nil {
		return nil, NewSyncError(SyncReasonDecode, err)
	}
	return &result, nil
}

func decodeCostsResult(raw any) (CostsResult, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return CostsResult{}, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return CostsResult{}, err
	}
	if object == nil {
		return CostsResult{}, fmt.Errorf("成本响应必须是 JSON 对象")
	}
	usage, usageOK := object["usage"]
	keys, keysOK := object["keys"]
	if !usageOK || !keysOK || strings.TrimSpace(string(usage)) == "null" || strings.TrimSpace(string(keys)) == "null" {
		return CostsResult{}, fmt.Errorf("成本响应缺少 usage 或 keys 数组")
	}
	var usageItems []MonthlyCostItem
	if err := json.Unmarshal(usage, &usageItems); err != nil {
		return CostsResult{}, fmt.Errorf("无效 usage 数组: %w", err)
	}
	var keyItems []KeyInfo
	if err := json.Unmarshal(keys, &keyItems); err != nil {
		return CostsResult{}, fmt.Errorf("无效 keys 数组: %w", err)
	}
	var result CostsResult
	if err := json.Unmarshal(data, &result); err != nil {
		return CostsResult{}, err
	}
	result.Usage = usageItems
	result.Keys = keyItems
	return result, nil
}

func (c *Client) GetUsageHistory(workspaceID string, page int) ([]UsageRecord, error) {
	raw, err := c.rpc(FnUsageHistory, []any{workspaceID, page})
	if retryableRPCError(err) {
		time.Sleep(500 * time.Millisecond)
		raw, err = c.rpc(FnUsageHistory, []any{workspaceID, page})
	}
	if err != nil {
		if page != 0 || SyncErrorReasonOf(err) == SyncReasonDecode {
			return nil, err
		}
		html, htmlErr := c.fetchUsageHTML(workspaceID)
		if htmlErr == nil {
			records := parseUsageHTML(html)
			if len(records) > 0 {
				return records, nil
			}
		}
		return nil, err
	}
	records, err := decodeUsageRecords(raw)
	if err != nil {
		return nil, NewSyncError(SyncReasonDecode, err)
	}
	return records, nil
}

func decodeUsageRecords(raw any) ([]UsageRecord, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "[") {
		return decodeUsageRecordArray(data)
	}

	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	for _, key := range []string{"data", "records", "usage"} {
		value, ok := wrapper[key]
		if !ok || strings.TrimSpace(string(value)) == "null" {
			continue
		}
		return decodeUsageRecordArray(value)
	}
	return nil, fmt.Errorf("无法识别 OpenCode usage history 响应")
}

func decodeUsageRecordArray(data []byte) ([]UsageRecord, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	records := make([]UsageRecord, 0, len(items))
	for _, item := range items {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			if err == nil {
				err = fmt.Errorf("usage record 不能为 null")
			}
			return nil, err
		}
		usageLike := false
		for _, key := range []string{
			"workspaceID", "workspaceId", "timeCreated", "timeUpdated", "model", "provider",
			"inputTokens", "outputTokens", "reasoningTokens", "cacheReadTokens",
			"cacheWrite5mTokens", "cacheWrite1hTokens", "cost", "keyID", "keyId",
			"sessionID", "enrichment",
		} {
			if _, ok := object[key]; ok {
				usageLike = true
				break
			}
		}
		idValue, hasID := object["id"]
		var id string
		if hasID {
			if err := json.Unmarshal(idValue, &id); err != nil {
				if !usageLike {
					continue
				}
				return nil, fmt.Errorf("usage record id 类型无效")
			}
		}
		if !usageLike && (!hasID || !strings.HasPrefix(id, "usg_")) {
			continue
		}
		if !strings.HasPrefix(id, "usg_") {
			return nil, fmt.Errorf("usage record %q 的 id 无效", id)
		}
		var record UsageRecord
		if err := json.Unmarshal(item, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return validateUsageRecords(records)
}

func validateUsageRecords(records []UsageRecord) ([]UsageRecord, error) {
	out := make([]UsageRecord, 0, len(records))
	for _, record := range records {
		if !strings.HasPrefix(record.ID, "usg_") {
			return nil, fmt.Errorf("usage record %q 的 id 无效", record.ID)
		}
		if strings.TrimSpace(record.TimeCreated) == "" {
			return nil, fmt.Errorf("usage record %s 缺少 timeCreated", record.ID)
		}
		if strings.TrimSpace(record.Model) == "" || strings.TrimSpace(record.Provider) == "" {
			return nil, fmt.Errorf("usage record %s 缺少 model 或 provider", record.ID)
		}
		out = append(out, record)
	}
	return out, nil
}

func (c *Client) fetchUsageHTML(workspaceID string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, "https://opencode.ai/workspace/"+url.PathEscape(workspaceID)+"/usage", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Cookie", c.buildCookieHeader())
	request.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTML fetch HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

func parseUsageHTML(html string) []UsageRecord {
	html = regexp.MustCompile(`new Date\("([^"]+)"\)`).ReplaceAllString(html, `"$1"`)
	objects := regexp.MustCompile(`(?s)\{[^{}]*"?id"?\s*:\s*"usg_[^"]+"[^{}]*\}`).FindAllString(html, -1)
	records := make([]UsageRecord, 0, len(objects))
	seen := make(map[string]bool)
	for _, object := range objects {
		record := UsageRecord{
			ID:                 jsString(object, "id"),
			WorkspaceID:        jsString(object, "workspaceID"),
			TimeCreated:        jsString(object, "timeCreated"),
			TimeUpdated:        jsString(object, "timeUpdated"),
			Model:              jsString(object, "model"),
			Provider:           jsString(object, "provider"),
			InputTokens:        jsNumber(object, "inputTokens"),
			OutputTokens:       jsNumber(object, "outputTokens"),
			ReasoningTokens:    jsNumber(object, "reasoningTokens"),
			CacheReadTokens:    jsNumber(object, "cacheReadTokens"),
			CacheWrite5mTokens: jsNumber(object, "cacheWrite5mTokens"),
			CacheWrite1hTokens: jsNumber(object, "cacheWrite1hTokens"),
			Cost:               jsNumber(object, "cost"),
			KeyID:              jsString(object, "keyID"),
		}
		if record.WorkspaceID == "" {
			record.WorkspaceID = jsString(object, "workspaceId")
		}
		if record.TimeUpdated == "" {
			record.TimeUpdated = record.TimeCreated
		}
		if record.KeyID == "" {
			record.KeyID = jsString(object, "keyId")
		}
		if sessionID := jsString(object, "sessionID"); sessionID != "" {
			record.SessionID = &sessionID
		}
		if record.ID != "" && !seen[record.ID] {
			seen[record.ID] = true
			records = append(records, record)
		}
	}
	sortUsage(records)
	validated, err := validateUsageRecords(records)
	if err != nil {
		return []UsageRecord{}
	}
	return validated
}

func jsString(object, key string) string {
	match := regexp.MustCompile(`"?` + regexp.QuoteMeta(key) + `"?\s*:\s*"([^"]*)"`).FindStringSubmatch(object)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func jsNumber(object, key string) float64 {
	match := regexp.MustCompile(`"?` + regexp.QuoteMeta(key) + `"?\s*:\s*(-?[0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(object)
	if len(match) < 2 {
		return 0
	}
	value, _ := strconv.ParseFloat(match[1], 64)
	return value
}

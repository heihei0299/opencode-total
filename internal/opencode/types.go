package opencode

import "errors"

type UsageRecord struct {
	ID                 string  `json:"id"`
	WorkspaceID        string  `json:"workspaceID"`
	TimeCreated        string  `json:"timeCreated"`
	TimeUpdated        string  `json:"timeUpdated"`
	TimeDeleted        *string `json:"timeDeleted"`
	Model              string  `json:"model"`
	Provider           string  `json:"provider"`
	InputTokens        float64 `json:"inputTokens"`
	OutputTokens       float64 `json:"outputTokens"`
	ReasoningTokens    float64 `json:"reasoningTokens"`
	CacheReadTokens    float64 `json:"cacheReadTokens"`
	CacheWrite5mTokens float64 `json:"cacheWrite5mTokens"`
	CacheWrite1hTokens float64 `json:"cacheWrite1hTokens"`
	Cost               float64 `json:"cost"`
	KeyID              string  `json:"keyID"`
	SessionID          *string `json:"sessionID"`
	Enrichment         any     `json:"enrichment"`
}

type MonthlyCostItem struct {
	Date      *string `json:"date"`
	Model     string  `json:"model"`
	TotalCost float64 `json:"totalCost"` // 比例缩放或直接浮点数
	KeyID     string  `json:"keyId"`
	Plan      string  `json:"plan"`
}

type KeyInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CostsResult struct {
	Usage []MonthlyCostItem `json:"usage"`
	Keys  []KeyInfo         `json:"keys"`
}

type WorkspaceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	WorkspaceID string `json:"workspaceID,omitempty"`
}

type SyncOptions struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	Full        bool   `json:"full,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type SyncStatus string

const (
	SyncComplete SyncStatus = "complete"
	SyncPartial  SyncStatus = "partial"
	SyncFailed   SyncStatus = "failed"
)

type SyncErrorReason string

const (
	SyncReasonConfiguration  SyncErrorReason = "configuration"
	SyncReasonInvalidInput   SyncErrorReason = "invalid_input"
	SyncReasonConflict       SyncErrorReason = "conflict"
	SyncReasonWorkspace      SyncErrorReason = "workspace"
	SyncReasonRemote         SyncErrorReason = "remote"
	SyncReasonNetwork        SyncErrorReason = "network"
	SyncReasonAuthentication SyncErrorReason = "authentication"
	SyncReasonNotFound       SyncErrorReason = "not_found"
	SyncReasonServer         SyncErrorReason = "server"
	SyncReasonHTTP           SyncErrorReason = "http"
	SyncReasonDecode         SyncErrorReason = "decode"
	SyncReasonIncomplete     SyncErrorReason = "incomplete"
	SyncReasonStorage        SyncErrorReason = "storage"
	SyncReasonInternal       SyncErrorReason = "internal"
)

type SyncError struct {
	Reason SyncErrorReason
	Err    error
}

func (e *SyncError) Error() string { return e.Err.Error() }
func (e *SyncError) Unwrap() error { return e.Err }

func NewSyncError(reason SyncErrorReason, err error) error {
	if err == nil {
		return nil
	}
	var existing *SyncError
	if errors.As(err, &existing) {
		return err
	}
	return &SyncError{Reason: reason, Err: err}
}

func SyncErrorReasonOf(err error) SyncErrorReason {
	var syncErr *SyncError
	if errors.As(err, &syncErr) {
		return syncErr.Reason
	}
	return SyncReasonInternal
}

type SyncResult struct {
	Added          int             `json:"added"`
	Updated        int             `json:"updated"`
	Pages          int             `json:"pages"`
	ElapsedMs      int64           `json:"elapsedMs"`
	LastSyncedTime string          `json:"lastSyncedTime"`
	Status         SyncStatus      `json:"status"`
	Reason         SyncErrorReason `json:"reason,omitempty"`
	Warnings       []string        `json:"warnings,omitempty"`
}

type HistoryFilter struct {
	Model     string `json:"model,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Since     string `json:"since,omitempty"`
	Until     string `json:"until,omitempty"`
}

type OpencodeTotals struct {
	Requests    int     `json:"requests"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cacheRead"`
	CacheWrite  float64 `json:"cacheWrite"`
	Reasoning   float64 `json:"reasoning"`
	TotalTokens float64 `json:"totalTokens"`
	Cost        float64 `json:"cost"`
}

type AuditDiff struct {
	Requests int     `json:"requests"`
	Tokens   float64 `json:"tokens"`
	Cost     float64 `json:"cost"`
}

type AuditDiffRate struct {
	Requests float64 `json:"requests"`
	Tokens   float64 `json:"tokens"`
	Cost     float64 `json:"cost"`
}

type LocalTotals struct {
	Requests    int     `json:"requests"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cacheRead"`
	CacheWrite  float64 `json:"cacheWrite"`
	Reasoning   float64 `json:"reasoning"`
	TotalTokens float64 `json:"totalTokens"`
	Cost        float64 `json:"cost"`
}

type AuditResult struct {
	Year           int            `json:"year"`
	Month          int            `json:"month"`
	LocalTotals    LocalTotals    `json:"localTotals"`
	OpencodeTotals OpencodeTotals `json:"opencodeTotals"`
	Diff           AuditDiff      `json:"diff"`
	DiffRate       AuditDiffRate  `json:"diffRate"`
	Comparison     string         `json:"comparison"`
}

package syncer

import (
	"fmt"
	"strings"
	"time"

	"github.com/heihei0299/opencode-analyzer/internal/credentials"
	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

type Options struct {
	Auth      string
	Workspace string
	DataDir   string
	Full      bool
	Limit     int
}

func failedResult(reason opencode.SyncErrorReason, err error) (opencode.SyncResult, error) {
	err = opencode.NewSyncError(reason, err)
	return opencode.SyncResult{Status: opencode.SyncFailed, Reason: opencode.SyncErrorReasonOf(err)}, err
}

func Run(options Options) (opencode.SyncResult, error) {
	creds, err := credentials.Load(credentials.Input{
		Auth:      options.Auth,
		Workspace: options.Workspace,
		DataDir:   options.DataDir,
	})
	if err != nil {
		return failedResult(opencode.SyncReasonConfiguration, err)
	}
	storage := opencode.NewStorage(creds.DataDir)
	unlock, err := storage.Lock()
	if err != nil {
		reason := opencode.SyncReasonStorage
		if strings.Contains(err.Error(), "同步进行中") {
			reason = opencode.SyncReasonConflict
		}
		return failedResult(reason, err)
	}
	defer unlock()

	client := opencode.NewClient(creds.Auth)
	workspace := creds.Workspace
	if workspace == "" {
		workspaces, err := client.GetWorkspaces()
		if err != nil {
			return failedResult(opencode.SyncReasonRemote, err)
		}
		if len(workspaces) == 0 {
			return failedResult(opencode.SyncReasonWorkspace, fmt.Errorf("无法自动发现工作区，请传入 --workspace 或设置 OPENCODE_WORKSPACE_ID"))
		}
		if len(workspaces) > 1 {
			return failedResult(opencode.SyncReasonWorkspace, fmt.Errorf("检测到多个工作区，请传入 --workspace 或设置 OPENCODE_WORKSPACE_ID"))
		}
		workspace = workspaces[0].ID
		if workspace == "" {
			workspace = workspaces[0].WorkspaceID
		}
		if workspace == "" {
			return failedResult(opencode.SyncReasonWorkspace, fmt.Errorf("工作区 ID 为空，无法同步"))
		}
	}

	now := time.Now()
	result, err := storage.Sync(client, opencode.SyncOptions{
		WorkspaceID: workspace,
		Full:        options.Full,
		Limit:       options.Limit,
	})
	if err != nil {
		result.Status = opencode.SyncFailed
		if result.Reason == "" {
			result.Reason = opencode.SyncErrorReasonOf(err)
		}
		return result, err
	}

	costs, costErr := client.GetMonthlyCosts(workspace, now.Year(), int(now.Month()))
	if costErr != nil {
		result.Status = opencode.SyncPartial
		result.Warnings = append(result.Warnings, fmt.Sprintf("月度成本未更新: %v", costErr))
	} else if costs == nil {
		result.Status = opencode.SyncPartial
		result.Warnings = append(result.Warnings, "月度成本未更新: OpenCode 未返回数据")
	} else if err := storage.SaveCosts(now.Year(), int(now.Month()), *costs); err != nil {
		result.Status = opencode.SyncPartial
		result.Warnings = append(result.Warnings, fmt.Sprintf("月度成本未更新: %v", err))
	}
	return result, nil
}

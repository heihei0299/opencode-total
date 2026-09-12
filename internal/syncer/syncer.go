package syncer

import (
	"fmt"
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

func Run(options Options) (opencode.SyncResult, error) {
	creds, err := credentials.Load(credentials.Input{
		Auth:      options.Auth,
		Workspace: options.Workspace,
		DataDir:   options.DataDir,
	})
	if err != nil {
		return opencode.SyncResult{}, err
	}
	storage := opencode.NewStorage(creds.DataDir)
	unlock, err := storage.Lock()
	if err != nil {
		return opencode.SyncResult{}, err
	}
	defer unlock()

	client := opencode.NewClient(creds.Auth)
	workspace := creds.Workspace
	if workspace == "" {
		workspaces, err := client.GetWorkspaces()
		if err != nil {
			return opencode.SyncResult{}, err
		}
		if len(workspaces) == 0 {
			return opencode.SyncResult{}, fmt.Errorf("无法自动发现工作区，请传入 --workspace 或设置 OPENCODE_WORKSPACE_ID")
		}
		if len(workspaces) > 1 {
			return opencode.SyncResult{}, fmt.Errorf("检测到多个工作区，请传入 --workspace 或设置 OPENCODE_WORKSPACE_ID")
		}
		workspace = workspaces[0].ID
		if workspace == "" {
			workspace = workspaces[0].WorkspaceID
		}
		if workspace == "" {
			return opencode.SyncResult{}, fmt.Errorf("工作区 ID 为空，无法同步")
		}
	}

	now := time.Now()
	if costs, err := client.GetMonthlyCosts(workspace, now.Year(), int(now.Month())); err == nil && costs != nil {
		if err := storage.SaveCosts(now.Year(), int(now.Month()), *costs); err != nil {
			return opencode.SyncResult{}, err
		}
	}
	result, err := storage.Sync(client, opencode.SyncOptions{
		WorkspaceID: workspace,
		Full:        options.Full,
		Limit:       options.Limit,
	})
	if err != nil {
		return opencode.SyncResult{}, err
	}
	return result, nil
}

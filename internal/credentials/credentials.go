package credentials

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

type Input struct {
	Auth      string
	Workspace string
	DataDir   string
}

type Credentials struct {
	Auth      string
	Workspace string
	DataDir   string
}

// Load resolves CLI values over process environment over local .env files.
func Load(input Input) (Credentials, error) {
	dataDir := opencode.ResolveDataDir(input.DataDir, os.Getenv("OPENCODE_DATA_DIR"))

	dotEnv := loadDotEnv(dataDir)
	auth := strings.TrimSpace(input.Auth)
	if auth == "" {
		auth = strings.TrimSpace(os.Getenv("OPENCODE_AUTH"))
	}
	if auth == "" {
		auth = strings.TrimSpace(dotEnv["OPENCODE_AUTH"])
	}
	if auth == "" {
		return Credentials{}, fmt.Errorf("缺少认证信息，请设置 OPENCODE_AUTH 环境变量或传 --auth")
	}

	workspace := strings.TrimSpace(input.Workspace)
	if workspace == "" {
		workspace = strings.TrimSpace(os.Getenv("OPENCODE_WORKSPACE_ID"))
	}
	if workspace == "" {
		workspace = strings.TrimSpace(os.Getenv("OPENCODE_WORKSPACE"))
	}
	if workspace == "" {
		workspace = strings.TrimSpace(dotEnv["OPENCODE_WORKSPACE_ID"])
		if workspace == "" {
			workspace = strings.TrimSpace(dotEnv["OPENCODE_WORKSPACE"])
		}
	}

	return Credentials{Auth: auth, Workspace: workspace, DataDir: dataDir}, nil
}

func loadDotEnv(dataDir string) map[string]string {
	paths := []string{filepath.Join(mustGetwd(), ".env")}
	dataEnv := filepath.Join(dataDir, ".env")
	if dataEnv != paths[0] {
		paths = append(paths, dataEnv)
	}
	out := make(map[string]string)
	for _, path := range paths {
		for key, value := range readEnvFile(path) {
			out[key] = value
		}
	}
	return out
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func readEnvFile(path string) map[string]string {
	file, err := os.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if key != "" {
			out[key] = value
		}
	}
	return out
}

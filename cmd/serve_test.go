package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServeCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("未注册 serve 命令: %v", err)
	}
	if cmd.Name() != "serve" {
		t.Fatalf("命令名 = %q，期望 serve", cmd.Name())
	}
}

func TestReadmeHasOperatorInstructions(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件")
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "README.md"))
	if err != nil {
		t.Fatalf("读取 README: %v", err)
	}
	text := string(body)

	required := []string{
		".env.example",
		"SESSION_SECRET",
		"至少 32",
		"CREDENTIAL_KEY",
		"openssl rand -base64 32",
		"sql/schema.sql",
		"已有表会按同一脚本补齐列、索引、外键",
		"脚本里没有的列、索引、外键会保留",
		"MYSQL_PREFIX",
		"make build-cli-mac",
		"./cli serve",
		"http://127.0.0.1:8080",
		"HTTPS_PROXY",
		"登录信息",
		"目标仓库",
		"同步任务",
		"阿里云",
	}
	for _, s := range required {
		if !strings.Contains(text, s) {
			t.Errorf("README 缺少 %q", s)
		}
	}

	forbidden := []string{
		"hub-mirror",
		"GITHUB_TOKEN",
		"GITHUB_HOST",
		"REDIS_HOST",
		"DOCKER_USERNAME",
		"DOCKER_PASSWORD",
		"sync:task",
		"PROXY_HOST=",
	}
	for _, s := range forbidden {
		if strings.Contains(text, s) {
			t.Errorf("README 不应再包含旧步骤 %q", s)
		}
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CREDENTIAL_KEY=") && trimmed != "CREDENTIAL_KEY=" {
			t.Errorf("README 不得放入 CREDENTIAL_KEY 样例值: %s", trimmed)
		}
	}
}

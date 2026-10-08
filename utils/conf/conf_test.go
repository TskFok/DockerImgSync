package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TskFok/DockerImgSync/app/global"
)

func TestConfigPathFromExecutable(t *testing.T) {
	exe := filepath.Join("opt", "app", "cli")
	got := ConfigPathFromExecutable(exe)
	want := filepath.Join("opt", "app", ".env")
	if got != want {
		t.Fatalf("配置路径 = %s，期望 %s", got, want)
	}
}

func TestEnvFilePathUsesExecutableDirectory(t *testing.T) {
	got, err := EnvFilePath()
	if err != nil {
		t.Fatalf("解析 env 路径失败: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("获取可执行文件失败: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatalf("解析可执行文件路径失败: %v", err)
	}
	want := filepath.Join(filepath.Dir(resolved), ".env")
	if got != want {
		t.Fatalf("env 路径 = %s，期望 %s", got, want)
	}
}

func TestLoadConfigReadsValuesFromEnvFile(t *testing.T) {
	resetGlobals()
	t.Cleanup(resetGlobals)

	path := writeEnv(t, validEnv())
	if err := LoadConfig(path); err != nil {
		t.Fatalf("读取 env 失败: %v", err)
	}

	assertEqual(t, "MysqlDsn", global.MysqlDsn, "user:pass@tcp(127.0.0.1:3306)/sync_task?charset=utf8mb4&parseTime=True&loc=Local")
	assertEqual(t, "MysqlPrefix", global.MysqlPrefix, "img_")
	assertEqual(t, "AdminUsername", global.AdminUsername, "admin")
	assertEqual(t, "AdminPassword", global.AdminPassword, "admin-pass")
	assertEqual(t, "SessionSecret", global.SessionSecret, "session-secret-must-be-32-characters-min")
	assertEqual(t, "HTTPAddr", global.HTTPAddr, ":9090")
	if string(global.CredentialKey) != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("CredentialKey = %q", string(global.CredentialKey))
	}
}

func TestLoadConfigDefaultsHTTPAddr(t *testing.T) {
	resetGlobals()
	t.Cleanup(resetGlobals)

	body := validEnv()
	body = replaceLine(body, "HTTP_ADDR=:9090", "HTTP_ADDR=")
	path := writeEnv(t, body)
	if err := LoadConfig(path); err != nil {
		t.Fatalf("读取 env 失败: %v", err)
	}
	assertEqual(t, "HTTPAddr", global.HTTPAddr, ":8080")
}

func TestLoadConfigRejectsMissingAdmin(t *testing.T) {
	resetGlobals()
	t.Cleanup(resetGlobals)

	body := replaceLine(validEnv(), "ADMIN_USERNAME=admin", "ADMIN_USERNAME=")
	err := LoadConfig(writeEnv(t, body))
	if err == nil {
		t.Fatal("缺少管理员用户名时应返回错误")
	}
}

func TestLoadConfigRejectsShortSessionSecret(t *testing.T) {
	resetGlobals()
	t.Cleanup(resetGlobals)

	body := replaceLine(validEnv(), "SESSION_SECRET=session-secret-must-be-32-characters-min", "SESSION_SECRET=short")
	err := LoadConfig(writeEnv(t, body))
	if err == nil {
		t.Fatal("过短的 SESSION_SECRET 应返回错误")
	}
}

func TestLoadConfigRejectsBadCredentialKey(t *testing.T) {
	resetGlobals()
	t.Cleanup(resetGlobals)

	body := replaceLine(validEnv(), "CREDENTIAL_KEY=YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=", "CREDENTIAL_KEY=bm90LTMy")
	err := LoadConfig(writeEnv(t, body))
	if err == nil {
		t.Fatal("长度不对的 CREDENTIAL_KEY 应返回错误")
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	err := LoadConfig(filepath.Join(t.TempDir(), ".env"))
	if err == nil {
		t.Fatal("缺少 env 文件时应返回错误")
	}
}

func validEnv() string {
	return "" +
		"MYSQL_DSN=\"user:pass@tcp(127.0.0.1:3306)/sync_task?charset=utf8mb4&parseTime=True&loc=Local\"\n" +
		"MYSQL_PREFIX=img_\n" +
		"ADMIN_USERNAME=admin\n" +
		"ADMIN_PASSWORD=admin-pass\n" +
		"SESSION_SECRET=session-secret-must-be-32-characters-min\n" +
		"CREDENTIAL_KEY=YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=\n" +
		"HTTP_ADDR=:9090\n"
}

func writeEnv(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写入测试 env 失败: %v", err)
	}
	return path
}

func replaceLine(body, old, newLine string) string {
	return strings.ReplaceAll(body, old, newLine)
}

func resetGlobals() {
	global.MysqlDsn = ""
	global.MysqlPrefix = ""
	global.AdminUsername = ""
	global.AdminPassword = ""
	global.SessionSecret = ""
	global.CredentialKey = nil
	global.HTTPAddr = ""
}

func assertEqual(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %q，期望 %q", name, got, want)
	}
}

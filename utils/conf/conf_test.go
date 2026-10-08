package conf

import (
	"os"
	"path/filepath"
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

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "" +
		"MYSQL_DSN=\"user:pass@tcp(127.0.0.1:3306)/sync_task?charset=utf8mb4&parseTime=True&loc=Local\"\n" +
		"MYSQL_PREFIX=img_\n" +
		"DOCKER_HOST=https://hub.docker.com\n" +
		"DOCKER_USERNAME=docker-user\n" +
		"DOCKER_PASSWORD=docker-pass\n" +
		"GITHUB_HOST=https://api.github.com/repos/example/hub-mirror/issues\n" +
		"GITHUB_TOKEN=token-value\n" +
		"PROXY_HOST=http://127.0.0.1:7890\n" +
		"REDIS_HOST=127.0.0.1:6379\n" +
		"REDIS_USER=default\n" +
		"REDIS_PASSWORD=redis-pass\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试 env 失败: %v", err)
	}

	if err := LoadConfig(path); err != nil {
		t.Fatalf("读取 env 失败: %v", err)
	}

	assertEqual(t, "MysqlDsn", global.MysqlDsn, "user:pass@tcp(127.0.0.1:3306)/sync_task?charset=utf8mb4&parseTime=True&loc=Local")
	assertEqual(t, "MysqlPrefix", global.MysqlPrefix, "img_")
	assertEqual(t, "DockerHost", global.DockerHost, "https://hub.docker.com")
	assertEqual(t, "DockerUsername", global.DockerUsername, "docker-user")
	assertEqual(t, "DockerPassword", global.DockerPassword, "docker-pass")
	assertEqual(t, "GithubHost", global.GithubHost, "https://api.github.com/repos/example/hub-mirror/issues")
	assertEqual(t, "GithubToken", global.GithubToken, "token-value")
	assertEqual(t, "ProxyHost", global.ProxyHost, "http://127.0.0.1:7890")
	assertEqual(t, "RedisHost", global.RedisHost, "127.0.0.1:6379")
	assertEqual(t, "RedisUser", global.RedisUser, "default")
	assertEqual(t, "RedisPassword", global.RedisPassword, "redis-pass")
}

func TestLoadConfigMissingFile(t *testing.T) {
	err := LoadConfig(filepath.Join(t.TempDir(), ".env"))
	if err == nil {
		t.Fatal("缺少 env 文件时应返回错误")
	}
}

func TestLoadConfigAllowsEmptyValues(t *testing.T) {
	resetGlobals()
	t.Cleanup(resetGlobals)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "" +
		"MYSQL_DSN=\n" +
		"MYSQL_PREFIX=\n" +
		"DOCKER_HOST=\n" +
		"DOCKER_USERNAME=\n" +
		"DOCKER_PASSWORD=\n" +
		"GITHUB_HOST=\n" +
		"GITHUB_TOKEN=\n" +
		"PROXY_HOST=\n" +
		"REDIS_HOST=\n" +
		"REDIS_USER=\n" +
		"REDIS_PASSWORD=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试 env 失败: %v", err)
	}

	if err := LoadConfig(path); err != nil {
		t.Fatalf("读取空值 env 失败: %v", err)
	}

	assertEqual(t, "MysqlDsn", global.MysqlDsn, "")
	assertEqual(t, "DockerPassword", global.DockerPassword, "")
	assertEqual(t, "GithubToken", global.GithubToken, "")
	assertEqual(t, "RedisPassword", global.RedisPassword, "")
}

func resetGlobals() {
	global.MysqlDsn = ""
	global.MysqlPrefix = ""
	global.DockerHost = ""
	global.DockerUsername = ""
	global.DockerPassword = ""
	global.GithubHost = ""
	global.GithubToken = ""
	global.ProxyHost = ""
	global.RedisUser = ""
	global.RedisPassword = ""
	global.RedisHost = ""
}

func assertEqual(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %q，期望 %q", name, got, want)
	}
}

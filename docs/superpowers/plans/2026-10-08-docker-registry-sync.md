# Docker 镜像仓库同步 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把现有 GitHub Issue 同步命令改成一个 Go 进程：登录后在页面管理仓库账号和同步任务，用仓库协议把镜像复制到目标仓库，digest 不变则跳过。

**Architecture:** `serve` 启动 HTTP 页面和进程内调度器。复制用 `go-containerregistry` 的 puller/pusher，源和目标各带各的认证。MySQL 存登录信息（AES-GCM）、目标仓库、任务和同步记录。单元测试使用假引擎和假存储，不访问真实仓库。

**Tech Stack:** Go 1.25、Cobra、Viper、GORM、MySQL、`github.com/go-chi/chi/v5`、`github.com/google/go-containerregistry`、`html/template`。`go-containerregistry` 使用 `@latest`（当前为 v0.22.1），因此 `go.mod` 的 Go 版本为 1.25.0。

**Spec:** `docs/superpowers/specs/2026-10-08-docker-registry-sync-design.md`

## Global Constraints

- 单个 Go 进程；不依赖本机 Docker、skopeo、GitHub、Redis。
- 只在源 manifest descriptor digest 变化时复制；不设置 platform，多架构索引原样复制。
- 复制必须走会上传 blob 的 puller.Get + pusher.Push。只 PUT manifest 不算完成。
- Docker Hub token 交换交给 go-containerregistry，禁止调用 `/v2/users/login`。
- 出站使用 `http.DefaultTransport`，遵循 `HTTPS_PROXY`。不再读取 `PROXY_HOST`。
- 同时同步最多 2 个。同一任务不并行。调度器每 15 秒一轮。
- 调度只用一条 JOIN 查询取出到期任务，再用一条 `WHERE id IN` 标为 running。循环里不查库。
- 登录密码 AES-256-GCM。nonce 12 字节放在密文前，整体标准 base64。`CREDENTIAL_KEY` 解码后必须是 32 字节。
- 管理员密码先 SHA-256，再用 `subtle.ConstantTimeCompare`。会话 cookie：HttpOnly、SameSite=Lax、HMAC-SHA256、24 小时。
- 所有 POST 校验 CSRF，失败返回 400。日志和页面不出现密码、密钥、Authorization。
- 检查间隔 0 表示只手动同步；大于 0 时最小 60 秒。
- 源镜像必须有 tag。默认目标仓库名是源路径最后一段。
- 目标仓库地址不含协议和路径。命名空间不含 `/`。
- 失败保留旧 digest，下次仍按间隔运行，不立刻重试。
- 阿里云仓库需事先建好，程序不创建仓库。
- `sql/schema.sql` 表名不带前缀。GORM `SingularTable` 加上 `MYSQL_PREFIX`。
- 登录信息和目标仓库外键 `ON DELETE RESTRICT`。`sync_log.sync_task_id` 为 `ON DELETE CASCADE`。
- 测试夹具只用明显假值，不写真实密钥。
- 页面为中文。

---

### Task 1: 配置改为 Web 所需项，并删除旧同步链路

**Files:**
- Modify: `app/global/global.go`
- Modify: `utils/conf/conf.go`
- Modify: `utils/conf/conf_test.go`
- Modify: `bootstrap/init.go`
- Modify: `cmd/root.go`
- Modify: `.env.example`
- Delete: `cmd/syncTask.go`
- Delete: `service/DockerApi/api.go`
- Delete: `service/GithubApi/api.go`
- Delete: `utils/cache/init.go`
- Delete: `utils/cache/client.go`
- Delete: `utils/curl/get.go`
- Delete: `utils/curl/post.go`
- Delete: `app/model/dockerImage.go`
- Delete: `app/model/issue.go`

**Interfaces:**
- Consumes: 现有 `conf.LoadConfig(path string) error`
- Produces: `global.MysqlDsn`、`global.MysqlPrefix`、`global.AdminUsername`、`global.AdminPassword`、`global.SessionSecret`、`global.CredentialKey []byte`、`global.HTTPAddr`。`LoadConfig` 在必填项缺失或 `CREDENTIAL_KEY` 非法时返回错误。空的 `HTTP_ADDR` 变成 `:8080`。

- [ ] **Step 1: 把配置测试改成新字段**

用下面内容覆盖 `utils/conf/conf_test.go`：

```go
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
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./utils/conf/ -count=1`

Expected: FAIL，`AdminUsername` 为空或 `global.AdminUsername` 未定义。

- [ ] **Step 3: 实现新配置并删除旧链路**

`app/global/global.go`：

```go
package global

import "gorm.io/gorm"

var DataBase *gorm.DB
var MysqlDsn string
var MysqlPrefix string
var AdminUsername string
var AdminPassword string
var SessionSecret string
var CredentialKey []byte
var HTTPAddr string
```

`utils/conf/conf.go` 的 `LoadConfig` 在读完文件后：

```go
global.MysqlDsn = v.GetString("mysql_dsn")
global.MysqlPrefix = v.GetString("mysql_prefix")
global.AdminUsername = v.GetString("admin_username")
global.AdminPassword = v.GetString("admin_password")
global.SessionSecret = v.GetString("session_secret")
global.HTTPAddr = v.GetString("http_addr")
if global.HTTPAddr == "" {
	global.HTTPAddr = ":8080"
}
if global.MysqlDsn == "" || global.AdminUsername == "" || global.AdminPassword == "" {
	return fmt.Errorf("MYSQL_DSN、ADMIN_USERNAME、ADMIN_PASSWORD 不能为空")
}
if len(global.SessionSecret) < 32 {
	return fmt.Errorf("SESSION_SECRET 至少 32 个字符")
}
key, err := base64.StdEncoding.DecodeString(v.GetString("credential_key"))
if err != nil || len(key) != 32 {
	return fmt.Errorf("CREDENTIAL_KEY 必须是 32 字节的标准 base64")
}
global.CredentialKey = key
return nil
```

补上 `"encoding/base64"` 导入。删除对 Docker、GitHub、Proxy、Redis 的赋值。

`bootstrap/init.go`：

```go
package bootstrap

import (
	"github.com/TskFok/DockerImgSync/app/global"
	"github.com/TskFok/DockerImgSync/utils/conf"
	"github.com/TskFok/DockerImgSync/utils/database"
)

func Init() {
	conf.InitConfig()
	global.DataBase = database.InitMysql()
}
```

`cmd/root.go` 的 `Use` 改为 `cli`，`Short` 改为 `镜像同步`。

`.env.example`：

```text
# 复制为本文件同目录下的 .env，并放在打包后的二进制文件旁边。
# 含特殊字符的值请用双引号包裹。
MYSQL_DSN=
MYSQL_PREFIX=
ADMIN_USERNAME=
ADMIN_PASSWORD=
SESSION_SECRET=
CREDENTIAL_KEY=
HTTP_ADDR=:8080
```

删除本任务 Files 里列出的旧文件。若目录变空，一并删除空目录。然后：

Run: `go mod tidy`

- [ ] **Step 4: 测试通过且整个模块可编译**

Run: `go test ./utils/conf/ -count=1 && go test ./... -count=1`

Expected: PASS。`go.mod` 不再 require `github.com/redis/go-redis/v9`。

- [ ] **Step 5: Commit**

```bash
git add app/global/global.go utils/conf bootstrap/init.go cmd/root.go cmd/syncTask.go service utils/cache utils/curl app/model .env.example go.mod go.sum
git commit -m "$(cat <<'EOF'
改为 Web 服务所需配置，并移除 GitHub 同步链路。

EOF
)"
```

### Task 2: 仓库密码加密

**Files:**
- Create: `utils/crypto/crypto.go`
- Test: `utils/crypto/crypto_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `func Encrypt(key []byte, plaintext string) (string, error)`
  - `func Decrypt(key []byte, encoded string) (string, error)`
  - 密文为标准 base64(12 字节 nonce ‖ GCM 密文)

- [ ] **Step 1: 写失败测试**

```go
package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte("a"), 32)
	encoded, err := Encrypt(key, "registry-pass")
	if err != nil {
		t.Fatal(err)
	}
	if encoded == "registry-pass" || encoded == "" {
		t.Fatalf("密文不应等于原文: %s", encoded)
	}
	got, err := Decrypt(key, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got != "registry-pass" {
		t.Fatalf("解密 = %s", got)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	key := bytes.Repeat([]byte("a"), 32)
	encoded, err := Encrypt(key, "registry-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(bytes.Repeat([]byte("b"), 32), encoded); err == nil {
		t.Fatal("错误密钥应解密失败")
	}
}

func TestEncryptRejectsBadKeyLength(t *testing.T) {
	if _, err := Encrypt([]byte("short"), "x"); err == nil {
		t.Fatal("密钥长度不对时应失败")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./utils/crypto/ -count=1`

Expected: FAIL，`Encrypt` 未定义。

- [ ] **Step 3: 实现 AES-GCM**

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("密钥长度必须是 32 字节: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(key []byte, encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密文过短")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
```

- [ ] **Step 4: 测试通过**

Run: `go test ./utils/crypto/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add utils/crypto
git commit -m "$(cat <<'EOF'
用 AES-GCM 加密仓库密码。

EOF
)"
```

### Task 3: 表结构、模型与纯校验

**Files:**
- Create: `sql/schema.sql`
- Create: `app/model/credential.go`
- Create: `app/model/registry.go`
- Create: `app/model/sync_task.go`
- Create: `app/model/sync_log.go`
- Create: `app/model/rules.go`
- Test: `app/model/rules_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - 模型 `Credential`、`Registry`、`SyncTask`、`SyncLog`。不定义 `TableName()`，交给现有 `SingularTable` 和 `MYSQL_PREFIX`。
  - `var ErrInUse = errors.New("仍被引用，不能删除")`
  - `var ErrTaskRunning = errors.New("任务正在同步，不能删除")`
  - `func CanDeleteCredential(registryCount, taskCount int) error`
  - `func CanDeleteRegistry(taskCount int) error`
  - `func CanDeleteTask(lastStatus string) error`
  - `func ValidateAddress(address string) error`
  - `func ValidateNamespace(namespace string) error`
  - `func ValidateInterval(seconds int) error`
  - 错误原文：`地址不能带协议或路径`、`命名空间不能含 /`、`检查间隔至少 60 秒`

- [ ] **Step 1: 写失败测试**

```go
package model

import "testing"

func TestCanDeleteCredential(t *testing.T) {
	if err := CanDeleteCredential(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := CanDeleteCredential(1, 0); err != ErrInUse {
		t.Fatalf("got %v", err)
	}
	if err := CanDeleteCredential(0, 2); err != ErrInUse {
		t.Fatalf("got %v", err)
	}
}

func TestCanDeleteRegistry(t *testing.T) {
	if err := CanDeleteRegistry(0); err != nil {
		t.Fatal(err)
	}
	if err := CanDeleteRegistry(1); err != ErrInUse {
		t.Fatalf("got %v", err)
	}
}

func TestCanDeleteTask(t *testing.T) {
	if err := CanDeleteTask("success"); err != nil {
		t.Fatal(err)
	}
	if err := CanDeleteTask("running"); err != ErrTaskRunning {
		t.Fatalf("got %v", err)
	}
}

func TestValidateAddress(t *testing.T) {
	if err := ValidateAddress("registry.cn-hangzhou.aliyuncs.com"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAddress("127.0.0.1:5000"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "https://registry.example.com", "registry.example.com/v2"} {
		if err := ValidateAddress(bad); err == nil || err.Error() != "地址不能带协议或路径" {
			t.Fatalf("%q got %v", bad, err)
		}
	}
}

func TestValidateNamespace(t *testing.T) {
	if err := ValidateNamespace("myns"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "a/b"} {
		if err := ValidateNamespace(bad); err == nil || err.Error() != "命名空间不能含 /" {
			t.Fatalf("%q got %v", bad, err)
		}
	}
}

func TestValidateInterval(t *testing.T) {
	if err := ValidateInterval(0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInterval(60); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []int{-1, 1, 59} {
		if err := ValidateInterval(bad); err == nil || err.Error() != "检查间隔至少 60 秒" {
			t.Fatalf("%d got %v", bad, err)
		}
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./app/model/ -count=1`

Expected: FAIL，函数未定义。

- [ ] **Step 3: 实现校验、模型和建表 SQL**

`app/model/rules.go`：

```go
package model

import (
	"errors"
	"strings"
)

var ErrInUse = errors.New("仍被引用，不能删除")
var ErrTaskRunning = errors.New("任务正在同步，不能删除")

func CanDeleteCredential(registryCount, taskCount int) error {
	if registryCount > 0 || taskCount > 0 {
		return ErrInUse
	}
	return nil
}

func CanDeleteRegistry(taskCount int) error {
	if taskCount > 0 {
		return ErrInUse
	}
	return nil
}

func CanDeleteTask(lastStatus string) error {
	if lastStatus == "running" {
		return ErrTaskRunning
	}
	return nil
}

func ValidateAddress(address string) error {
	if address == "" || strings.Contains(address, "://") || strings.Contains(address, "/") {
		return errors.New("地址不能带协议或路径")
	}
	return nil
}

func ValidateNamespace(namespace string) error {
	if namespace == "" || strings.Contains(namespace, "/") {
		return errors.New("命名空间不能含 /")
	}
	return nil
}

func ValidateInterval(seconds int) error {
	if seconds == 0 || seconds >= 60 {
		return nil
	}
	return errors.New("检查间隔至少 60 秒")
}
```

模型使用 `time.Time` 和 GORM 自动时间，不嵌入 `BaseModel`，也不定义 `TableName()`。

`app/model/credential.go`：

```go
package model

import "time"

type Credential struct {
	ID                int32     `gorm:"column:id;primaryKey;autoIncrement"`
	Name              string    `gorm:"column:name;type:varchar(255);not null"`
	Username          string    `gorm:"column:username;type:varchar(255);not null"`
	PasswordEncrypted string    `gorm:"column:password_encrypted;type:text;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime"`
}
```

`app/model/registry.go` 的 `Registry` 含 `ID`、`Name`、`Address`、`Namespace`、`CredentialID int32`、`CreatedAt`、`UpdatedAt`，列名与 spec 一致。

`app/model/sync_task.go`：

```go
type SyncTask struct {
	ID                 int32      `gorm:"column:id;primaryKey;autoIncrement"`
	Name               string     `gorm:"column:name"`
	SourceImage        string     `gorm:"column:source_image"`
	SourceCredentialID *int32     `gorm:"column:source_credential_id"`
	RegistryID         int32      `gorm:"column:registry_id"`
	DestRepository     string     `gorm:"column:dest_repository"`
	DestTag            string     `gorm:"column:dest_tag"`
	IntervalSeconds    int        `gorm:"column:interval_seconds"`
	Enabled            bool       `gorm:"column:enabled"`
	LastDigest         string     `gorm:"column:last_digest"`
	LastStatus         string     `gorm:"column:last_status"`
	LastError          string     `gorm:"column:last_error"`
	LastSyncedAt       *time.Time `gorm:"column:last_synced_at"`
	NextRunAt          *time.Time `gorm:"column:next_run_at"`
	CreatedAt          time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}
```

`app/model/sync_log.go` 的 `SyncLog` 含 `ID`、`SyncTaskID`、`Trigger`、`Status`、`SourceDigest`、`Message`、`StartedAt`、`FinishedAt`。

`sql/schema.sql`：

```sql
-- 表名不带前缀。若 MYSQL_PREFIX 非空，建表时加上同一个前缀。

CREATE TABLE `credential` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `username` varchar(255) NOT NULL,
  `password_encrypted` text NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_credential_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE `registry` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `address` varchar(255) NOT NULL,
  `namespace` varchar(255) NOT NULL,
  `credential_id` int NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_registry_name` (`name`),
  CONSTRAINT `fk_registry_credential` FOREIGN KEY (`credential_id`) REFERENCES `credential` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE `sync_task` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `source_image` varchar(512) NOT NULL,
  `source_credential_id` int NULL,
  `registry_id` int NOT NULL,
  `dest_repository` varchar(255) NOT NULL DEFAULT '',
  `dest_tag` varchar(255) NOT NULL DEFAULT '',
  `interval_seconds` int NOT NULL DEFAULT 0,
  `enabled` tinyint NOT NULL DEFAULT 1,
  `last_digest` varchar(255) NOT NULL DEFAULT '',
  `last_status` varchar(32) NOT NULL DEFAULT 'idle',
  `last_error` text NOT NULL,
  `last_synced_at` datetime NULL,
  `next_run_at` datetime NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_sync_task_due` (`enabled`, `next_run_at`),
  CONSTRAINT `fk_sync_task_registry` FOREIGN KEY (`registry_id`) REFERENCES `registry` (`id`) ON DELETE RESTRICT,
  CONSTRAINT `fk_sync_task_source_credential` FOREIGN KEY (`source_credential_id`) REFERENCES `credential` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE `sync_log` (
  `id` int NOT NULL AUTO_INCREMENT,
  `sync_task_id` int NOT NULL,
  `trigger` varchar(16) NOT NULL,
  `status` varchar(16) NOT NULL,
  `source_digest` varchar(255) NOT NULL DEFAULT '',
  `message` text NOT NULL,
  `started_at` datetime NOT NULL,
  `finished_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `fk_sync_log_task` FOREIGN KEY (`sync_task_id`) REFERENCES `sync_task` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

- [ ] **Step 4: 测试通过**

Run: `go test ./app/model/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add sql/schema.sql app/model
git commit -m "$(cat <<'EOF'
添加同步任务的表结构和删除校验。

EOF
)"
```

### Task 4: 解析源镜像并拼接目标地址

**Files:**
- Create: `service/sync/ref.go`
- Test: `service/sync/ref_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `func ParseSource(image string) (normalized, repository, tag string, err error)`
  - `func DestRef(address, namespace, repository, tag string) (string, error)`
  - 没有 tag 时错误文本为 `源镜像必须包含 tag`
  - `nginx:latest` → normalized `docker.io/library/nginx:latest`，repository `nginx`，tag `latest`
  - `ghcr.io/org/team/app:1` → repository `app`

- [ ] **Step 1: 写失败测试**

```go
package sync

import "testing"

func TestParseSourceOfficialImage(t *testing.T) {
	normalized, repo, tag, err := ParseSource("nginx:latest")
	if err != nil {
		t.Fatal(err)
	}
	if normalized != "docker.io/library/nginx:latest" || repo != "nginx" || tag != "latest" {
		t.Fatalf("got %s %s %s", normalized, repo, tag)
	}
}

func TestParseSourceNestedPathUsesLastSegment(t *testing.T) {
	_, repo, tag, err := ParseSource("ghcr.io/org/team/app:1")
	if err != nil {
		t.Fatal(err)
	}
	if repo != "app" || tag != "1" {
		t.Fatalf("got %s %s", repo, tag)
	}
}

func TestParseSourceRejectsDigestOnly(t *testing.T) {
	_, _, _, err := ParseSource("nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || err.Error() != "源镜像必须包含 tag" {
		t.Fatalf("got %v", err)
	}
}

func TestDestRefUsesOverride(t *testing.T) {
	got, err := DestRef("registry.cn-hangzhou.aliyuncs.com", "myns", "nginx", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if got != "registry.cn-hangzhou.aliyuncs.com/myns/nginx:latest" {
		t.Fatalf("got %s", got)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/sync/ -count=1`

Expected: FAIL，`ParseSource` 未定义。

- [ ] **Step 3: 实现解析**

依赖：`go get github.com/google/go-containerregistry@latest`

用 `name.ParseReference(image, name.WeakValidation)`。类型不是 `name.Tag` 时返回 `errors.New("源镜像必须包含 tag")`。`normalized` 用 `ref.Name()`，并把仓库主机前缀 `index.docker.io` 换成 `docker.io`，因此 `nginx:latest` 得到 `docker.io/library/nginx:latest`。仓库路径 `ref.Context().RepositoryStr()` 按 `/` 分割，取最后一段作为 `repository`。`tag` 用 `tagRef.TagStr()`。

`DestRef` 在地址、命名空间、仓库名、tag 任一为空时返回错误。否则返回 `address + "/" + namespace + "/" + repository + ":" + tag`。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/sync/ -count=1`

Expected: PASS。`nginx:latest` 的规范化结果必须是 `docker.io/library/nginx:latest`。

- [ ] **Step 5: Commit**

```bash
git add service/sync go.mod go.sum
git commit -m "$(cat <<'EOF'
解析源镜像并生成目标仓库引用。

EOF
)"
```

### Task 5: 同步执行逻辑

**Files:**
- Create: `service/sync/engine.go`
- Create: `service/sync/runner.go`
- Test: `service/sync/runner_test.go`

**Interfaces:**
- Consumes: `ParseSource`、`DestRef`
- Produces:

```go
type Auth struct {
	Username string
	Password string
}

type Engine interface {
	Digest(ctx context.Context, ref string, auth *Auth) (string, error)
	Copy(ctx context.Context, src, dst string, srcAuth, dstAuth *Auth) error
}

type Task struct {
	ID                int32
	SourceImage       string
	DestRepository    string
	DestTag           string
	RegistryAddress   string
	RegistryNamespace string
	SourceAuth        *Auth
	DestAuth          *Auth
	LastDigest        string
	LastStatus        string
	IntervalSeconds   int
}

type Result struct {
	Status         string
	ObservedDigest string
	Message        string
	LastDigest     string
	NextRunAt      *time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
}

func Run(ctx context.Context, eng Engine, task Task, now func() time.Time) Result
```

`now == nil` 时用 `time.Now`。`DestRepository` 或 `DestTag` 为空时用 `ParseSource` 的默认值。

- [ ] **Step 1: 写失败测试**

`service/sync/runner_test.go`：

```go
package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeEngine struct {
	digest    string
	digestErr error
	copyCount int
	dst       string
	srcAuth   *Auth
	dstAuth   *Auth
}

func (f *fakeEngine) Digest(ctx context.Context, ref string, auth *Auth) (string, error) {
	f.srcAuth = auth
	return f.digest, f.digestErr
}

func (f *fakeEngine) Copy(ctx context.Context, src, dst string, srcAuth, dstAuth *Auth) error {
	f.copyCount++
	f.dst = dst
	f.srcAuth = srcAuth
	f.dstAuth = dstAuth
	return nil
}

func fixed(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestRunSkipsSameDigest(t *testing.T) {
	eng := &fakeEngine{digest: "sha256:same"}
	got := Run(context.Background(), eng, Task{
		SourceImage:       "nginx:latest",
		RegistryAddress:   "registry.example.com",
		RegistryNamespace: "ns",
		DestAuth:          &Auth{Username: "dest-user", Password: "registry-pass"},
		LastDigest:        "sha256:same",
	}, fixed(time.Unix(100, 0)))
	if got.Status != "skipped" || got.Message != "无变化" || got.LastDigest != "sha256:same" || eng.copyCount != 0 {
		t.Fatalf("got %+v copy %d", got, eng.copyCount)
	}
}

func TestRunCopiesWhenDigestChanges(t *testing.T) {
	eng := &fakeEngine{digest: "sha256:new"}
	now := time.Unix(100, 0)
	got := Run(context.Background(), eng, Task{
		SourceImage:       "nginx:latest",
		RegistryAddress:   "registry.example.com",
		RegistryNamespace: "ns",
		DestAuth:          &Auth{Username: "dest-user", Password: "registry-pass"},
		LastDigest:        "sha256:old",
		IntervalSeconds:   60,
	}, fixed(now))
	if got.Status != "success" || got.LastDigest != "sha256:new" || eng.copyCount != 1 {
		t.Fatalf("got %+v copy %d", got, eng.copyCount)
	}
	if eng.dst != "registry.example.com/ns/nginx:latest" || eng.srcAuth != nil || eng.dstAuth.Username != "dest-user" {
		t.Fatalf("dst %s src %#v dest %#v", eng.dst, eng.srcAuth, eng.dstAuth)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(now.Add(60*time.Second)) || !got.StartedAt.Equal(now) || !got.FinishedAt.Equal(now) {
		t.Fatalf("time %+v", got)
	}
}

func TestRunRedactsPasswordOnDigestError(t *testing.T) {
	eng := &fakeEngine{digestErr: errors.New("login failed for registry-pass")}
	got := Run(context.Background(), eng, Task{
		SourceImage:       "nginx:latest",
		RegistryAddress:   "registry.example.com",
		RegistryNamespace: "ns",
		SourceAuth:        &Auth{Username: "src", Password: "registry-pass"},
		DestAuth:          &Auth{Username: "dest-user", Password: "registry-pass"},
		LastDigest:        "sha256:old",
	}, fixed(time.Unix(100, 0)))
	if got.Status != "failed" || got.LastDigest != "sha256:old" || eng.copyCount != 0 {
		t.Fatalf("got %+v", got)
	}
	if got.Message == "" || strings.Contains(got.Message, "registry-pass") || !strings.Contains(got.Message, "[redacted]") {
		t.Fatalf("message %s", got.Message)
	}
}

func TestRunRequiresDestAuth(t *testing.T) {
	eng := &fakeEngine{}
	got := Run(context.Background(), eng, Task{SourceImage: "nginx:latest"}, fixed(time.Unix(100, 0)))
	if got.Status != "failed" || got.Message != "目标仓库缺少登录信息" || eng.copyCount != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestRunManualHasNoNextTime(t *testing.T) {
	eng := &fakeEngine{digest: "sha256:same"}
	got := Run(context.Background(), eng, Task{
		SourceImage:       "nginx:latest",
		RegistryAddress:   "registry.example.com",
		RegistryNamespace: "ns",
		DestAuth:          &Auth{Username: "dest-user"},
		LastDigest:        "sha256:same",
		IntervalSeconds:   0,
	}, fixed(time.Unix(100, 0)))
	if got.NextRunAt != nil {
		t.Fatal("间隔为 0 时不应有下次运行时间")
	}
}

```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/sync/ -count=1 -run 'TestRun'`

Expected: FAIL，`Run` 未定义。

- [ ] **Step 3: 实现 Run**

顺序：调用 `now()` 得到开始时间；`DestAuth == nil` 则失败返回；`ParseSource`；空的目标仓库名和 tag 用默认值；`DestRef`；`Digest`。任一错误用 `safeMessage` 后失败返回，且 `LastDigest` 仍是入参。digest 相同则 skipped。否则 `Copy`，失败同样保留旧 digest，但 `ObservedDigest` 填刚读到的值。成功则 `LastDigest` 改为新 digest，`Message` 为空。结束时再调用 `now()`。间隔大于 0 时 `NextRunAt = finished + interval`。

```go
func safeMessage(err error, auths ...*Auth) string {
	msg := err.Error()
	for _, auth := range auths {
		if auth != nil && auth.Password != "" {
			msg = strings.ReplaceAll(msg, auth.Password, "[redacted]")
		}
	}
	return msg
}
```

- [ ] **Step 4: 测试通过**

Run: `go test ./service/sync/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/sync
git commit -m "$(cat <<'EOF'
按 digest 决定跳过或复制镜像。

EOF
)"
```

### Task 6: 到期判断与进程重启

**Files:**
- Create: `service/sync/due.go`
- Test: `service/sync/due_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `func IsDue(enabled bool, intervalSeconds int, nextRunAt *time.Time, lastStatus string, now time.Time) bool`
  - `const InterruptedMessage = "进程重启，同步中断"`
  - `func Interrupted(lastStatus string) (status, message, trigger string, changed bool)`
  - running 变为 `failed` / `进程重启，同步中断` / `startup`

- [ ] **Step 1: 写失败测试**

```go
package sync

import (
	"testing"
	"time"
)

func TestIsDue(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	if !IsDue(true, 60, &past, "success", now) {
		t.Fatal("到期任务应被选中")
	}
	if !IsDue(true, 60, &now, "failed", now) {
		t.Fatal("刚好到期应被选中")
	}
	if IsDue(false, 60, &past, "success", now) ||
		IsDue(true, 0, &past, "success", now) ||
		IsDue(true, 60, &future, "success", now) ||
		IsDue(true, 60, &past, "running", now) ||
		IsDue(true, 60, nil, "idle", now) {
		t.Fatal("未启用、间隔为 0、未到期、正在运行或没有下次时间的任务不应到期")
	}
}

func TestInterrupted(t *testing.T) {
	status, message, trigger, changed := Interrupted("running")
	if !changed || status != "failed" || message != InterruptedMessage || trigger != "startup" {
		t.Fatalf("got %s %s %s %v", status, message, trigger, changed)
	}
	if _, _, _, changed := Interrupted("success"); changed {
		t.Fatal("非 running 不应改写")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/sync/ -count=1 -run 'TestIsDue|TestInterrupted'`

Expected: FAIL

- [ ] **Step 3: 实现**

`IsDue`：未启用、间隔小于等于 0、状态为 `running`、或 `nextRunAt == nil` 时返回 false。`!nextRunAt.After(now)` 返回 true。

`Interrupted`：仅当状态是 `running` 时 `changed` 为 true，并返回失败状态、`InterruptedMessage` 和 `startup`。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/sync/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/sync/due.go service/sync/due_test.go
git commit -m "$(cat <<'EOF'
判断任务是否到期，并定义重启中断结果。

EOF
)"
```

### Task 7: 真实仓库复制

**Files:**
- Create: `service/sync/crane.go`
- Test: `service/sync/crane_test.go`

**Interfaces:**
- Consumes: `Engine`、`Auth`
- Produces:
  - `func NewCrane() *Crane`
  - `func authenticator(auth *Auth) authn.Authenticator`（同包测试可调用）
  - `(*Crane).Digest` 使用 `remote.NewPuller` + `Head`
  - `(*Crane).Copy` 使用源认证的 puller `Get`，再用目标认证的 pusher `Push`
  - 两个方向都带 `remote.WithTransport(http.DefaultTransport)`
  - `auth == nil` 或用户名为空时使用 `authn.Anonymous`
  - 不调用 `crane.Copy`，因为它的 pull 和 push 共用同一组 Remote 选项

- [ ] **Step 1: 写失败测试**

```go
package sync

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
)

func TestAuthenticatorAnonymous(t *testing.T) {
	if authenticator(nil) != authn.Anonymous {
		t.Fatal("nil 认证应为匿名")
	}
	if authenticator(&Auth{}) != authn.Anonymous {
		t.Fatal("空用户名应为匿名")
	}
}

func TestAuthenticatorBasic(t *testing.T) {
	got := authenticator(&Auth{Username: "dest-user", Password: "registry-pass"})
	basic, ok := got.(*authn.Basic)
	if !ok || basic.Username != "dest-user" || basic.Password != "registry-pass" {
		t.Fatalf("got %#v", got)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/sync/ -count=1 -run TestAuthenticator`

Expected: FAIL

- [ ] **Step 3: 实现 Crane**

```go
func authenticator(auth *Auth) authn.Authenticator {
	if auth == nil || auth.Username == "" {
		return authn.Anonymous
	}
	return &authn.Basic{Username: auth.Username, Password: auth.Password}
}

func (c *Crane) Digest(ctx context.Context, ref string, auth *Auth) (string, error) {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return "", err
	}
	puller, err := remote.NewPuller(remote.WithAuth(authenticator(auth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return "", err
	}
	desc, err := puller.Head(ctx, parsed)
	if err != nil {
		return "", err
	}
	return desc.Digest.String(), nil
}

func (c *Crane) Copy(ctx context.Context, src, dst string, srcAuth, dstAuth *Auth) error {
	srcRef, err := name.ParseReference(src)
	if err != nil {
		return err
	}
	dstRef, err := name.ParseReference(dst)
	if err != nil {
		return err
	}
	puller, err := remote.NewPuller(remote.WithAuth(authenticator(srcAuth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return err
	}
	desc, err := puller.Get(ctx, srcRef)
	if err != nil {
		return err
	}
	pusher, err := remote.NewPusher(remote.WithAuth(authenticator(dstAuth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return err
	}
	return pusher.Push(ctx, dstRef, desc)
}
```

不要设置 platform，也不要调用 `crane.Copy`。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/sync/ -count=1`

Expected: PASS。此任务不访问网络。

- [ ] **Step 5: Commit**

```bash
git add service/sync/crane.go service/sync/crane_test.go
git commit -m "$(cat <<'EOF'
用独立认证从源仓库复制镜像到目标仓库。

EOF
)"
```

### Task 8: 调度器、手动同步与 MySQL 存储

**Files:**
- Create: `service/sync/scheduler.go`
- Create: `service/sync/mysql_store.go`
- Test: `service/sync/scheduler_test.go`

**Interfaces:**
- Consumes: `Engine`、`Task`、`Result`、`Run`、`IsDue`、`Interrupted`、`utils/crypto.Decrypt`、`app/model`
- Produces:

```go
var ErrBusy = errors.New("任务正在同步")

type Store interface {
	ListDue(ctx context.Context, now time.Time) ([]Task, error)
	MarkRunning(ctx context.Context, ids []int32) error
	Finish(ctx context.Context, id int32, trigger string, result Result) error
	ResetRunning(ctx context.Context, now time.Time) error
	Get(ctx context.Context, id int32) (Task, error)
}

func NewScheduler(store Store, eng Engine) *Scheduler
func (s *Scheduler) Tick(ctx context.Context, now time.Time) error
func (s *Scheduler) SyncNow(ctx context.Context, id int32) error
func (s *Scheduler) Start(ctx context.Context) error
func NewMySQLStore(db *gorm.DB, key []byte) *MySQLStore
```

`const PollInterval = 15 * time.Second`。信号量容量 2。

- [ ] **Step 1: 写调度器失败测试**

`fakeStore` 记录 `listCalls`、`marked []int32`、`finished`。`ListDue` 返回预设任务。每条任务都要有 `DestAuth`、可解析的 `SourceImage`（`nginx:latest`）、目标地址和命名空间，否则 `Run` 会失败而不是 skipped。`Get` 按 id 返回任务，找不到则返回错误。假引擎的 digest 等于任务里的 `LastDigest`，所以结果是 `skipped`。

断言：

- 两个到期任务时，`MarkRunning` 收到 `[]int32{1, 2}` 一次。等待两个 `Finish`，trigger 都是 `schedule`，status 都是 `skipped`。
- `ListDue` 返回空切片时，`marked` 仍为空。
- `Get` 返回 `LastStatus: "running"` 时，`SyncNow` 返回 `ErrBusy`，且 `marked` 为空。
- `LastStatus` 为空闲时，`SyncNow` 在返回前已经 `MarkRunning([]int32{id})`，并且返回值是 nil。随后等待 `Finish` 的 trigger 为 `manual`。

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/sync/ -count=1 -run 'TestTick|TestSyncNow'`

Expected: FAIL

- [ ] **Step 3: 实现调度器和 MySQLStore**

`Tick`：`ListDue`；没有 id 就返回；一次 `MarkRunning(ids)`；每个任务 `go s.run(...)`。`run` 先占用容量为 2 的信号量，调用 `Run`，再 `Finish`。`Finish` 的错误只 `fmt.Println` 错误文本，不打印 `Auth`。

`SyncNow`：按任务 id 使用互斥锁。锁内 `Get`；状态是 `running` 返回 `ErrBusy`；否则 `MarkRunning([]int32{id})`，解锁后启动 `run`，trigger 为 `manual`。立即返回 nil。

`Start`：先 `ResetRunning`。失败则返回错误给调用方，不在库函数里 panic。然后每 `PollInterval` 调用 `Tick`，直到 `ctx.Done()`。

`MySQLStore.ListDue` 一条 SQL JOIN `registry`、目标 `credential`、左连接源 `credential`。条件与 `IsDue` 一致：`enabled = 1`、`interval_seconds > 0`、`next_run_at <= now`、`last_status <> 'running'`。在内存里 `crypto.Decrypt`，填 `SourceAuth`（源账号为空则为 nil）和 `DestAuth`。

`MarkRunning`：`WHERE id IN ?` 更新 `last_status = running`。空切片直接返回。

`Finish`：一个事务里更新该任务的 `last_status`、`last_digest`、`last_error`（成功或 skipped 时清空）、`last_synced_at`、`next_run_at`，并插入一条 `SyncLog`。

`ResetRunning`：一条查询取出 `last_status = running` 的 id；没有则返回。一条 `WHERE id IN` 更新为 `failed`，`last_error` 为 `InterruptedMessage`。再 `Create` 一批 `trigger = startup` 的失败日志。不要在循环里查库或逐条插入。

`Get` 使用和 `ListDue` 相同的 JOIN，按 id 取一条。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/sync/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/sync/scheduler.go service/sync/mysql_store.go service/sync/scheduler_test.go
git commit -m "$(cat <<'EOF'
在进程内调度到期任务，并支持立即同步。

EOF
)"
```

### Task 9: 管理员会话

**Files:**
- Create: `service/web/session.go`
- Create: `service/web/session_test.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `func CheckPassword(given, want string) bool`
  - `func SignSession(secret, user, csrf string, exp time.Time) (string, error)`
  - `func VerifySession(secret, cookie string, now time.Time) (user, csrf string, err error)`
  - `func NewCSRF() (string, error)`
  - cookie 载荷是 `base64.RawURLEncoding(json) + "." + hex(hmac-sha256)`
  - JSON 字段：`user`、`csrf`、`exp`（unix 秒）

- [ ] **Step 1: 写失败测试**

```go
package web

import (
	"testing"
	"time"
)

func TestCheckPassword(t *testing.T) {
	if !CheckPassword("admin-pass", "admin-pass") {
		t.Fatal("相同密码应通过")
	}
	if CheckPassword("admin-pass", "other-pass") {
		t.Fatal("不同密码应失败")
	}
}

func TestSignAndVerifySession(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	cookie, err := SignSession("session-secret-must-be-32-characters-min", "admin", "csrf-token", exp)
	if err != nil {
		t.Fatal(err)
	}
	user, csrf, err := VerifySession("session-secret-must-be-32-characters-min", cookie, time.Now())
	if err != nil || user != "admin" || csrf != "csrf-token" {
		t.Fatalf("got %s %s %v", user, csrf, err)
	}
	if _, _, err := VerifySession("session-secret-must-be-32-characters-min", cookie, exp.Add(time.Second)); err == nil {
		t.Fatal("过期会话应失败")
	}
	if _, _, err := VerifySession("session-secret-must-be-32-characters-min", "x"+cookie, time.Now()); err == nil {
		t.Fatal("篡改会话应失败")
	}
}

func TestNewCSRF(t *testing.T) {
	a, err := NewCSRF()
	if err != nil || a == "" {
		t.Fatal(err)
	}
	b, err := NewCSRF()
	if err != nil || a == b {
		t.Fatalf("两次 csrf 不应相同: %s %s", a, b)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/web/ -count=1`

Expected: FAIL

- [ ] **Step 3: 实现会话**

`CheckPassword` 对两边做 `sha256.Sum256`，再用 `subtle.ConstantTimeCompare`。

`SignSession` 用 HMAC-SHA256。`VerifySession` 用 `hmac.Equal` 比较签名，并检查 `exp` 晚于 `now`。

`NewCSRF` 读取 32 字节随机数，返回 hex。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/web/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/web/session.go service/web/session_test.go
git commit -m "$(cat <<'EOF'
添加管理员会话签名和密码比较。

EOF
)"
```

### Task 10: 登录与登录信息页面

**Files:**
- Create: `service/web/server.go`
- Create: `service/web/credential.go`
- Create: `service/web/templates/layout.html`
- Create: `service/web/templates/login.html`
- Create: `service/web/templates/credentials.html`
- Create: `service/web/templates/credential_form.html`
- Test: `service/web/server_test.go`
- Create: `service/web/mysql_store.go` 中的登录信息方法（文件在本任务创建，后续任务往同一文件追加仓库和任务方法）

**Interfaces:**
- Consumes: `CheckPassword`、`SignSession`、`VerifySession`、`NewCSRF`、`model.CanDeleteCredential`、`crypto.Encrypt`
- Produces:

```go
type Credential struct {
	ID       int32
	Name     string
	Username string
	Password string
}

type CredentialStore interface {
	ListCredentials(ctx context.Context) ([]Credential, error)
	GetCredential(ctx context.Context, id int32) (Credential, error)
	CreateCredential(ctx context.Context, c Credential) error
	UpdateCredential(ctx context.Context, c Credential) error
	DeleteCredential(ctx context.Context, id int32) error
}

type Deps struct {
	Store         CredentialStore
	AdminUser     string
	AdminPassword string
	SessionSecret string
}

func NewRouter(deps Deps) http.Handler
```

`GetCredential` 的 `Password` 留空，页面不回显密码。`UpdateCredential` 在 `Password == ""` 时不改密文。cookie 名 `session`，Path `/`，HttpOnly，SameSite=Lax，不设置 Secure。

- [ ] **Step 1: 写失败测试**

使用只实现 `CredentialStore` 的假存储：

- 无 cookie 访问 `GET /credentials` 返回 302，Location 为 `/login`。
- `POST /login` 密码错误返回 401。
- 正确用户名 `admin`、密码 `admin-pass` 后 Set-Cookie，再访问 `GET /credentials` 返回 200，正文含 `我的账号`。
- 已登录时 `POST /credentials/1/delete` 不带 csrf 返回 400。
- 假存储里该账号被引用时，带正确 csrf 的删除返回 200 页面（重新渲染列表）且正文含 `仍被引用，不能删除`，假存储的删除计数仍为 0。实现时让 handler 把 store 返回的 `ErrInUse` 显示出来；测试里的假 `DeleteCredential` 直接返回 `model.ErrInUse`。

登录前先 `GET /login` 取得 csrf。测试从 HTML 中取出 `name="csrf"` 的 value。

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/web/ -count=1 -run 'TestLogin|TestCredential'`

Expected: FAIL

- [ ] **Step 3: 实现路由和页面**

`GET /login` 签发 user 为空的 24 小时会话，只用于携带 csrf。`POST /login` 校验 csrf 和 `CheckPassword`。成功后签发 user 为管理员用户名的新 cookie，302 到 `/tasks`。`POST /logout` 清空 cookie 并跳到 `/login`。

除 `/login` 外，中间件要求会话 user 等于管理员用户名，否则 302 到 `/login`。所有 POST 先 `ParseForm`，再比较表单 `csrf` 与会话 csrf。`/login` 的 POST 在 handler 内做同样比较。

页面用 `embed` 加载 `templates/*.html`。布局含三个链接：`/credentials`、`/registries`、`/tasks`，以及退出表单。登录信息列表有新建链接。表单含名称、用户名、密码；编辑页说明密码留空表示不修改。删除按钮是带 csrf 的 POST。

`MySQLStore` 保存加密后的密码。删除前分别 `Count` 引用该 id 的 `registry` 和 `source_credential_id`，再调用 `CanDeleteCredential`。这是两次计数，不是循环查询。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/web/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/web
git commit -m "$(cat <<'EOF'
添加登录页和登录信息管理。

EOF
)"
```

### Task 11: 目标仓库页面

**Files:**
- Modify: `service/web/server.go`
- Create: `service/web/registry.go`
- Create: `service/web/templates/registries.html`
- Create: `service/web/templates/registry_form.html`
- Modify: `service/web/mysql_store.go`
- Test: `service/web/registry_test.go`

**Interfaces:**
- Consumes: `ValidateAddress`、`ValidateNamespace`、`CanDeleteRegistry`、Task 10 的 `Deps` 与会话
- Produces: 扩展 `Deps.Store` 为同时包含仓库方法的接口。若一个接口过大，就新增 `RegistryStore`，并由 `Deps` 增加字段 `Registries RegistryStore`。不要改 Task 10 已有方法的签名。

```go
type Registry struct {
	ID             int32
	Name           string
	Address        string
	Namespace      string
	CredentialID   int32
	CredentialName string
}

type RegistryStore interface {
	ListRegistries(ctx context.Context) ([]Registry, error)
	GetRegistry(ctx context.Context, id int32) (Registry, error)
	CreateRegistry(ctx context.Context, r Registry) error
	UpdateRegistry(ctx context.Context, r Registry) error
	DeleteRegistry(ctx context.Context, id int32) error
}
```

- [ ] **Step 1: 写失败测试**

已登录客户端：

- `POST /registries` 地址为 `https://registry.example.com` 时，响应正文含 `地址不能带协议或路径`，假存储创建次数为 0。
- 合法地址 `registry.example.com`、命名空间 `myns`、名称为 `杭州`、credential id 为 1 时，假存储收到这条记录。
- 删除被引用的仓库时正文含 `仍被引用，不能删除`。

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/web/ -count=1 -run TestRegistry`

Expected: FAIL

- [ ] **Step 3: 实现页面**

路由与 spec 一致。表单的登录信息是下拉框，选项来自 `ListCredentials`。保存前调用 `ValidateAddress` 和 `ValidateNamespace`。删除前 `Count` 引用该仓库的任务，再调用 `CanDeleteRegistry`。列表显示地址、命名空间和登录信息名称。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/web/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/web
git commit -m "$(cat <<'EOF'
添加目标仓库配置页面。

EOF
)"
```

### Task 12: 同步任务页面

**Files:**
- Modify: `service/web/server.go`
- Create: `service/web/task.go`
- Create: `service/web/templates/tasks.html`
- Create: `service/web/templates/task_form.html`
- Create: `service/web/templates/task_detail.html`
- Modify: `service/web/mysql_store.go`
- Test: `service/web/task_test.go`

**Interfaces:**
- Consumes: `ParseSource`、`ValidateInterval`、`CanDeleteTask`、`Scheduler.SyncNow`、`ErrBusy`
- Produces:

```go
type SyncTask struct {
	ID                 int32
	Name               string
	SourceImage        string
	SourceCredentialID *int32
	RegistryID         int32
	DestRepository     string
	DestTag            string
	IntervalSeconds    int
	Enabled            bool
	LastDigest         string
	LastStatus         string
	LastError          string
	LastSyncedAt       *time.Time
	NextRunAt          *time.Time
}

type SyncLog struct {
	Trigger        string
	Status         string
	SourceDigest   string
	Message        string
	StartedAt      time.Time
	FinishedAt     time.Time
}

type TaskStore interface {
	ListTasks(ctx context.Context) ([]SyncTask, error)
	GetTask(ctx context.Context, id int32) (SyncTask, error)
	ListLogs(ctx context.Context, taskID int32) ([]SyncLog, error)
	CreateTask(ctx context.Context, task SyncTask, now time.Time) error
	UpdateTask(ctx context.Context, task SyncTask, now time.Time) error
	DeleteTask(ctx context.Context, id int32) error
	SetEnabled(ctx context.Context, id int32, enabled bool, now time.Time) error
}

type Syncer interface {
	SyncNow(ctx context.Context, id int32) error
}
```

`Deps` 增加 `Tasks TaskStore` 和 `Syncer Syncer`。

- [ ] **Step 1: 写失败测试**

- 源镜像 `nginx@sha256:` 加 64 个 `a` 被拒绝，正文含 `源镜像必须包含 tag`。
- 间隔 30 被拒绝，正文含 `检查间隔至少 60 秒`。
- 合法任务 `nginx:latest`、目标仓库 id 1、间隔 60：假存储收到的 `SourceImage` 是 `docker.io/library/nginx:latest`，`NextRunAt` 非空。
- 间隔 0 的任务 `NextRunAt` 为空。
- 把间隔从 0 改成 60 时 `NextRunAt` 被设为当前时间；从 60 改成 0 时 `NextRunAt` 为空。
- `POST /tasks/1/sync` 调用 `Syncer.SyncNow`，响应是 302 到 `/tasks/1`。`SyncNow` 返回 `ErrBusy` 时详情正文含 `任务正在同步`。
- 状态为 `running` 的删除被拒绝，正文含 `任务正在同步，不能删除`。
- 关闭后再启用：`SetEnabled(true)` 收到的任务应在存储实现里把 `next_run_at` 设为 now。测试直接断言假 `SetEnabled` 被调用时 enabled 为 true；MySQL 实现单独用注释对照 spec，不在本测试连数据库。

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./service/web/ -count=1 -run TestTask`

Expected: FAIL

- [ ] **Step 3: 实现页面和保存规则**

创建和更新前：`ParseSource`，把源镜像存成 normalized 字符串；`ValidateInterval`。间隔大于 0 且是新建、或间隔从 0 变为大于 0、或重新启用时，`NextRunAt = now`。间隔改为 0 时 `NextRunAt = nil`。

目标仓库名和 tag 的占位说明是「与源相同」。列表显示 `last_status`；失败时显示 `last_error`。详情按存储返回的顺序展示日志，MySQL 实现 `ORDER BY id DESC`。

`POST /tasks/{id}/sync` 调用 `SyncNow` 后立刻 302，不等待复制结束。`ErrBusy` 时仍 302，并在详情查询参数 `msg` 中放 `任务正在同步`。

删除前 `GetTask`，再 `CanDeleteTask`。启用开关走 `POST /tasks/{id}/toggle`。

- [ ] **Step 4: 测试通过**

Run: `go test ./service/web/ ./service/sync/ ./app/model/ ./utils/... -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add service/web
git commit -m "$(cat <<'EOF'
添加同步任务页面和立即同步入口。

EOF
)"
```

### Task 13: 启动服务并改写说明

**Files:**
- Create: `cmd/serve.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `bootstrap.Init`、`sync.NewCrane`、`sync.NewMySQLStore`、`sync.NewScheduler`、`web.NewRouter`、`global` 配置
- Produces: 命令 `./cli serve`

- [ ] **Step 1: 写 README 中可人工核对的启动说明，并实现 serve**

`Deps` 的最终字段以 Task 12 为准：`Credentials`、`Registries`、`Tasks`、`Syncer`、`AdminUser`、`AdminPassword`、`SessionSecret`。一个 `*web.MySQLStore` 同时实现这三个 store 接口，`serve` 只构造一次。

`cmd/serve.go` 注册 `serve`。`Run` 里：

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
engine := syncsvc.NewCrane()
syncStore := syncsvc.NewMySQLStore(global.DataBase, global.CredentialKey)
sched := syncsvc.NewScheduler(syncStore, engine)
if err := sched.Start(ctx); err != nil {
	fmt.Println(err.Error())
	os.Exit(1)
}
store := web.NewMySQLStore(global.DataBase, global.CredentialKey)
router := web.NewRouter(web.Deps{
	Credentials:   store,
	Registries:    store,
	Tasks:         store,
	Syncer:        sched,
	AdminUser:     global.AdminUsername,
	AdminPassword: global.AdminPassword,
	SessionSecret: global.SessionSecret,
})
if err := http.ListenAndServe(global.HTTPAddr, router); err != nil {
	fmt.Println(err.Error())
	os.Exit(1)
}
```

`Start` 先执行 `ResetRunning` 并返回它的错误。ticker 放在 goroutine 里，`Start` 本身不阻塞。

`README.md` 替换为中文说明，包含：

- 复制 `.env.example` 为二进制旁边的 `.env`
- `SESSION_SECRET` 至少 32 字符；`CREDENTIAL_KEY` 用 `openssl rand -base64 32` 在本地生成，文档不放样例密钥
- 按 `sql/schema.sql` 建表；若 `MYSQL_PREFIX` 非空，表名加同一前缀
- `make build-cli-mac` 后执行 `./cli serve`
- 浏览器打开 `http://127.0.0.1:8080`
- 先建登录信息，再建目标仓库，再建同步任务
- 目标仓库要事先在阿里云控制台建好
- 需要代理时设置环境变量 `HTTPS_PROXY`，不要再写 `PROXY_HOST`
- 去掉 GitHub Issue、hub-mirror、Redis、Docker Hub 登录的旧步骤

- [ ] **Step 2: 编译并跑全部测试**

Run: `go test ./... -count=1 && go build -o /tmp/docker-img-sync-cli ./bin/cli/`

Expected: 测试 PASS，二进制生成。不要启动后去连真实 MySQL，除非本机已经有不含真实密码的测试库。没有测试库就只做编译。

- [ ] **Step 3: Commit**

```bash
git add cmd/serve.go README.md
git commit -m "$(cat <<'EOF'
提供 serve 命令并改写使用说明。

EOF
)"
```

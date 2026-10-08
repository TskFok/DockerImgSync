package conf

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TskFok/DockerImgSync/app/global"
	"github.com/spf13/viper"
)

const envFileName = ".env"

func InitConfig() {
	path, err := EnvFilePath()
	if err != nil {
		panic(err)
	}
	if err := LoadConfig(path); err != nil {
		panic(err)
	}
}

// EnvFilePath 返回打包后二进制文件同目录下的 .env 路径。
func EnvFilePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取可执行文件路径失败: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("解析可执行文件路径失败: %w", err)
	}
	return ConfigPathFromExecutable(resolved), nil
}

// ConfigPathFromExecutable 根据可执行文件路径得到同目录的 .env 路径。
func ConfigPathFromExecutable(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), envFileName)
}

// LoadConfig 从指定 env 文件读取配置并写入全局变量。
func LoadConfig(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("env")
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

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
}

package conf

import (
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
	global.DockerHost = v.GetString("docker_host")
	global.DockerUsername = v.GetString("docker_username")
	global.DockerPassword = v.GetString("docker_password")
	global.GithubHost = v.GetString("github_host")
	global.GithubToken = v.GetString("github_token")
	global.ProxyHost = v.GetString("proxy_host")
	global.RedisUser = v.GetString("redis_user")
	global.RedisPassword = v.GetString("redis_password")
	global.RedisHost = v.GetString("redis_host")
	return nil
}

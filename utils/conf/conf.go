package conf

import (
	"bytes"
	_ "embed"
	"github.com/TskFok/DockerImgSync/app/global"
	"github.com/spf13/viper"
)

//go:embed conf.yaml
var conf []byte

func InitConfig() {
	viper.SetConfigType("yaml")

	err := viper.ReadConfig(bytes.NewReader(conf))

	if nil != err {
		panic(err)
	}

	global.MysqlDsn = viper.Get("mysql.dsn").(string)
	global.MysqlPrefix = viper.Get("mysql.prefix").(string)
	global.DockerHost = viper.Get("docker.host").(string)
	global.DockerUsername = viper.Get("docker.username").(string)
	global.DockerPassword = viper.Get("docker.password").(string)
	global.GithubHost = viper.Get("github.host").(string)
	global.GithubToken = viper.Get("github.token").(string)
	global.ProxyHost = viper.Get("proxy.host").(string)
	global.RedisUser = viper.Get("redis.user").(string)
	global.RedisPassword = viper.Get("redis.password").(string)
	global.RedisHost = viper.Get("redis.host").(string)
}

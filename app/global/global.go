package global

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var RedisClient *redis.Client
var DataBase *gorm.DB
var MysqlDsn string
var MysqlPrefix string
var DockerUsername string
var DockerPassword string
var DockerHost string
var GithubHost string
var GithubToken string
var ProxyHost string
var RedisUser string
var RedisPassword string
var RedisHost string

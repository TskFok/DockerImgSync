package bootstrap

import (
	"github.com/TskFok/DockerImgSync/app/global"
	"github.com/TskFok/DockerImgSync/utils/cache"
	"github.com/TskFok/DockerImgSync/utils/conf"
	"github.com/TskFok/DockerImgSync/utils/database"
)

func Init() {
	conf.InitConfig()

	global.RedisClient = cache.InitRedis()
	global.DataBase = database.InitMysql()
}

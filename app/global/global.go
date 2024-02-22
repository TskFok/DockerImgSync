package global

import (
	"gorm.io/gorm"
)

var DataBase *gorm.DB
var MysqlDsn string
var MysqlPrefix string
var DockerUsername string
var DockerPassword string
var DockerHost string
var GithubHost string
var GithubToken string
var ProxyHost string

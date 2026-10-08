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

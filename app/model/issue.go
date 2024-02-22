package model

import (
	"fmt"
	"github.com/TskFok/DockerImgSync/app/global"
)

type issue struct {
	BaseModel
	Id         int32  `gorm:"column:id;type:INT(11);AUTO_INCREMENT;NOT NULL"`
	Namespace  string `gorm:"column:namespace;type:VARCHAR(255);NOT NULL"`
	Repository string `gorm:"column:repository;type:VARCHAR(255);NOT NULL"`
	Tag        string `gorm:"column:tag;type:VARCHAR(255);NOT NULL"`
	From       string `gorm:"column:from;type:VARCHAR(255);NOT NULL"`
	Url        string `gorm:"column:url;type:VARCHAR(255);NOT NULL"`
	HtmlUrl    string `gorm:"column:html_url;type:VARCHAR(255);NOT NULL"`
}

func NewIssue() *issue {
	return new(issue)
}

func (*issue) Create(is *issue) int32 {
	db := global.DataBase.Create(is)

	if db.Error != nil {
		fmt.Println(db.Error.Error())

		return 0
	}

	return is.Id
}

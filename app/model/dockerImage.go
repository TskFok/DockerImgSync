package model

import (
	"fmt"
	"github.com/TskFok/DockerImgSync/app/global"
	"time"
)

type dockerImage struct {
	BaseModel
	Id           int32     `gorm:"column:id;type:INT(11);AUTO_INCREMENT;NOT NULL"`
	Namespace    string    `gorm:"column:namespace;type:VARCHAR(255);NOT NULL"`
	Repository   string    `gorm:"column:repository;type:VARCHAR(255);NOT NULL"`
	Tag          string    `gorm:"column:tag;type:VARCHAR(255);NOT NULL"`
	From         string    `gorm:"column:from;type:VARCHAR(255);NOT NULL"`
	RepositoryId int32     `gorm:"column:repository_id;type:INT(11);NOT NULL"`
	LastUpdated  time.Time `gorm:"column:last_updated;type:DATETIME;NOT NULL"`
	TagStatus    string    `gorm:"column:tag_status;type:VARCHAR(255);NOT NULL"`
}

func NewDockerImage() *dockerImage {
	return new(dockerImage)
}

func (*dockerImage) Find(repositoryId int32) (d *dockerImage) {
	condition := make(map[string]any)
	condition["repository_id"] = repositoryId

	db := global.DataBase.Where(condition).First(&d)

	if db.Error != nil {
		fmt.Println(db.Error.Error())
		return nil
	}

	return d
}

func (*dockerImage) Create(di *dockerImage) int32 {
	db := global.DataBase.Create(di)

	if db.Error != nil {
		fmt.Println(db.Error.Error())

		return 0
	}

	return di.Id
}

func (*dockerImage) Update(condition map[string]interface{}, updates map[string]interface{}) bool {
	db := global.DataBase.Model(&dockerImage{}).Where(condition).Updates(updates)

	if db.Error != nil {
		fmt.Println(db.Error.Error())

		return false
	}

	return true
}

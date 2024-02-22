package DockerApi

import (
	"fmt"
	"github.com/TskFok/DockerImgSync/app/global"
	"github.com/TskFok/DockerImgSync/utils/curl"
	"net/http"
	"time"
)

type loginRes struct {
	Token string `json:"token,omitempty"`
}

type tagRes struct {
	Repository  int32     `json:"repository,omitempty"`
	LastUpdated time.Time `json:"last_updated"`
	TagStatus   string    `json:"tag_status,omitempty"`
}

func Login() string {
	body := make(map[string]interface{})
	body["username"] = global.DockerUsername
	body["password"] = global.DockerPassword
	header := http.Header{}
	header.Add("Content-Type", "application/json")

	login := &loginRes{}
	httpStatus := curl.Post(global.DockerHost+"/v2/users/login", body, header, login)

	if httpStatus != http.StatusOK {
		fmt.Println(httpStatus)
		fmt.Println("请求失败")

		return ""
	}

	return login.Token
}

func Detail(namespace, repository, tag, token string) *tagRes {
	header := http.Header{}
	header.Add("Authorization", "Bearer "+token)
	res := &tagRes{}

	url := global.DockerHost + "/v2/namespaces/" + namespace + "/repositories/" + repository + "/tags/" + tag
	httpStatus := curl.Get(url, res, header)

	if httpStatus != http.StatusOK {
		fmt.Println(httpStatus)
		fmt.Println("请求失败")

		return nil
	}

	return res
}

package GithubApi

import (
	"encoding/json"
	"fmt"
	"github.com/TskFok/DockerImgSync/app/global"
	"github.com/TskFok/DockerImgSync/utils/curl"
	"net/http"
)

type issues struct {
	Url     string `json:"url,omitempty"`
	HtmlUrl string `json:"html_url,omitempty"`
}

type requestBody struct {
	Platform  string   `json:"platform"`
	HubMirror []string `json:"hub-mirror,omitempty"`
}

func CreateIssues(from, namespace, repository, tag string) *issues {
	body := make(map[string]any)
	body["title"] = "[hub-mirror] 请求执行任务"

	//处理labels字段数据
	label := make([]string, 1)
	label[0] = "hub-mirror"
	body["labels"] = label

	//处理body字段数据
	hubMirror := make([]string, 1)
	hubMirror[0] = from + "/" + namespace + "/" + repository + ":" + tag
	rb := &requestBody{}
	rb.HubMirror = hubMirror
	rb.Platform = ""
	bd, _ := json.Marshal(rb)
	body["body"] = string(bd)

	//修改请求头,使用github的token
	header := http.Header{}
	header.Add("Accept", "application/vnd.github+json")
	header.Add("Authorization", "Bearer "+global.GithubToken)
	header.Add("X-GitHub-Api-Version", "2022-11-28")

	issue := &issues{}
	httpStatus := curl.Post(global.GithubHost, body, header, issue)

	if httpStatus != http.StatusCreated {
		fmt.Println(httpStatus)
		fmt.Println("请求失败")

		return nil
	}

	return issue
}

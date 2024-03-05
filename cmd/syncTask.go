package cmd

import (
	"fmt"
	"github.com/TskFok/DockerImgSync/app/model"
	"github.com/TskFok/DockerImgSync/service/DockerApi"
	"github.com/TskFok/DockerImgSync/service/GithubApi"
	"github.com/TskFok/DockerImgSync/utils/cache"
	"github.com/spf13/cobra"
)

var namespace string
var repository string
var tag string
var from string
var all int

// syncTaskCmd represents the syncTask command
var syncTaskCmd = &cobra.Command{
	Use:   "sync:task",
	Short: "同步docker images到阿里云镜像仓库",
	Long:  `同步docker images到阿里云镜像仓库`,
	Run: func(cmd *cobra.Command, args []string) {
		token := getToken()

		if all == 1 {
			dockerImage := model.NewDockerImage()

			list := dockerImage.FindAll()

			for _, v := range list {
				imgDetail := DockerApi.Detail(v.Namespace, v.Repository, v.Tag, token)

				namespace = v.Namespace
				repository = v.Repository
				tag = v.Tag
				from = v.From

				if imgDetail == nil {
					fmt.Println(namespace + "/" + repository + " 获取仓库详情失败")
					continue
				}

				if v.LastUpdated.Unix() == imgDetail.LastUpdated.Unix() {
					fmt.Println(namespace + "/" + repository + " 未找到新的变更")
					continue
				}

				//找到新更新
				condition := make(map[string]any)
				condition["id"] = v.Id

				update := make(map[string]any)
				update["tag_status"] = imgDetail.TagStatus
				update["last_updated"] = imgDetail.LastUpdated
				up := dockerImage.Update(condition, update)

				if !up {
					fmt.Println("更新失败")
				}

				createIssue()
			}

			return
		}

		detail := DockerApi.Detail(namespace, repository, tag, token)

		if detail == nil {
			fmt.Println("获取仓库详情失败")
			return
		}

		dockerImage := model.NewDockerImage()

		one := dockerImage.Find(detail.Repository)

		if one != nil {
			if one.LastUpdated.Unix() == detail.LastUpdated.Unix() {
				fmt.Println(namespace + "/" + repository + " 未找到新的变更")
				return
			}

			//找到新更新
			condition := make(map[string]any)
			condition["id"] = one.Id

			update := make(map[string]any)
			update["tag_status"] = detail.TagStatus
			update["last_updated"] = detail.LastUpdated
			up := dockerImage.Update(condition, update)

			if !up {
				fmt.Println("更新失败")
			}

			createIssue()
		} else {
			//新建记录
			newDockerImage := model.NewDockerImage()
			newDockerImage.TagStatus = detail.TagStatus
			newDockerImage.RepositoryId = detail.Repository
			newDockerImage.LastUpdated = detail.LastUpdated
			newDockerImage.Namespace = namespace
			newDockerImage.Repository = repository
			newDockerImage.From = from
			newDockerImage.Tag = tag
			dockerImage.Create(newDockerImage)

			createIssue()
		}
	},
}

func getToken() string {
	token := ""
	if cache.Has("docker_token") {
		token = cache.Get("docker_token")
	} else {
		token = DockerApi.Login()

		if token == "" {
			fmt.Println("获取docker hub token失败")
			return ""
		}

		cache.Set("docker_token", token, 3600)
	}

	return token
}

func createIssue() {
	//创建github issue,通过workflow自动同步到自己的仓库
	issue := GithubApi.CreateIssues(from, namespace, repository, tag)

	if nil == issue {
		fmt.Println(namespace + "/" + repository + " 创建github issue失败")
		return
	}

	issueModel := model.NewIssue()

	issueData := model.NewIssue()
	issueData.Url = issue.Url
	issueData.HtmlUrl = issue.HtmlUrl
	issueData.Namespace = namespace
	issueData.Repository = repository
	issueData.From = from
	issueData.Tag = tag
	id := issueModel.Create(issueData)

	if id == 0 {
		fmt.Println(namespace + "/" + repository + " 写入issues表失败")
	} else {
		fmt.Println(namespace + "/" + repository + " 写入issues表成功")
	}

	return
}

func init() {
	syncTaskCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "命名空间")
	syncTaskCmd.Flags().StringVarP(&repository, "repository", "r", "", "仓库名称")
	syncTaskCmd.Flags().StringVarP(&tag, "tag", "t", "", "镜像标签")
	syncTaskCmd.Flags().StringVarP(&from, "from", "f", "", "镜像来源")
	syncTaskCmd.Flags().IntVarP(&all, "all", "a", 0, "全部更新")
	rootCmd.AddCommand(syncTaskCmd)
}

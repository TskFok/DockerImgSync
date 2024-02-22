package main

import (
	"github.com/TskFok/DockerImgSync/bootstrap"
	"github.com/TskFok/DockerImgSync/cmd"
)

func main() {
	bootstrap.Init()

	cmd.Execute()
}

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/TskFok/DockerImgSync/app/global"
	syncsvc "github.com/TskFok/DockerImgSync/service/sync"
	"github.com/TskFok/DockerImgSync/service/web"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 Web 服务和同步调度器",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		engine := syncsvc.NewCrane()
		syncStore := syncsvc.NewMySQLStore(global.DataBase, global.CredentialKey)
		sched := syncsvc.NewScheduler(syncStore, engine)
		if err := sched.Start(ctx); err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		store := web.NewMySQLStore(global.DataBase, global.CredentialKey)
		router := web.NewRouter(web.Deps{
			Store:         store,
			Registries:    store,
			Tasks:         store,
			Syncer:        sched,
			AdminUser:     global.AdminUsername,
			AdminPassword: global.AdminPassword,
			SessionSecret: global.SessionSecret,
		})
		if err := http.ListenAndServe(global.HTTPAddr, router); err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

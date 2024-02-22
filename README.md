# 同步docker镜像到自己的仓库

``````
根据
https://github.com/togettoyou/hub-mirror
fork一个项目做好前置准备

配置文件
填写docker账号密码
填写github的token,host填写https://api.github.com/repos/****/hub-mirror/issues,****是用户名
如果网络受限可以添加proxy

使用方法
go run bin/cli/main.go sync:task --namespace="linuxserver" --repository="jackett" --tag="latest" --from="lscr.io"
``````

``````
创建数据库sync_task

创建表
CREATE TABLE `docker_image` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `namespace` varchar(255) NOT NULL COMMENT '命名空间',
  `repository` varchar(255) NOT NULL COMMENT '仓库名称',
  `tag` varchar(255) NOT NULL COMMENT '镜像标签',
  `from` varchar(255) NOT NULL COMMENT '镜像来源',
  `repository_id` int(11) NOT NULL COMMENT '存储库 ID',
  `last_updated` datetime NOT NULL COMMENT '上次更新的日期时间',
  `tag_status` varchar(255) NOT NULL COMMENT '标签在过去一个月内是否被推送或拉取',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `updated_at` datetime NOT NULL COMMENT '修改时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE `issue` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `namespace` varchar(255) NOT NULL COMMENT '命名空间',
  `repository` varchar(255) NOT NULL COMMENT '仓库名称',
  `tag` varchar(255) NOT NULL COMMENT '镜像标签',
  `from` varchar(255) NOT NULL COMMENT '镜像来源',
  `url` varchar(255) NOT NULL COMMENT '地址',
  `html_url` varchar(255) NOT NULL COMMENT 'html地址',
  `created_at` datetime NOT NULL COMMENT '创建时间',
  `updated_at` datetime NOT NULL COMMENT '修改时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
``````
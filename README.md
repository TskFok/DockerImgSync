# 同步docker镜像到自己的仓库

``````
根据
https://github.com/togettoyou/hub-mirror
fork一个项目做好前置准备

配置文件

打包后，在二进制文件同目录放置 `.env`（可复制仓库根目录的 `.env.example`）。程序只读取该文件，不再把配置编译进二进制。

```
MYSQL_DSN=
MYSQL_PREFIX=
DOCKER_HOST=https://hub.docker.com
DOCKER_USERNAME=
DOCKER_PASSWORD=
GITHUB_HOST=
GITHUB_TOKEN=
PROXY_HOST=
REDIS_HOST=127.0.0.1:6379
REDIS_USER=
REDIS_PASSWORD=
```

填写 docker 账号密码。
`GITHUB_HOST` 填写 `https://api.github.com/repos/****/hub-mirror/issues`，`****` 是用户名，并填写 `GITHUB_TOKEN`。
如果网络受限，可以填写 `PROXY_HOST`。
含特殊字符的值请用双引号包裹。

使用方法

先编译，并把 `.env` 放在生成的二进制旁边：

```
make build-cli-mac
./cli sync:task --namespace="linuxserver" --repository="jackett" --tag="latest" --from="lscr.io"
```

更新已有的全部任务：

```
./cli sync:task --all=1
```
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
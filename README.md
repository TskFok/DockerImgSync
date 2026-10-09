# Docker 镜像仓库同步

单个进程提供登录页面和定时同步，用仓库协议把镜像复制到目标仓库。不依赖本机 Docker、GitHub 或 Redis。

## 配置

1. 复制仓库根目录的 `.env.example`，保存为**打包后二进制文件同目录**下的 `.env`。程序只读取该文件。
2. 填写 `MYSQL_DSN`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`。
3. `SESSION_SECRET` 至少 32 个字符，用于签名会话 cookie。
4. `CREDENTIAL_KEY` 请在本地生成，不要把真实密钥写进文档或提交到仓库：

```
openssl rand -base64 32
```

解码后必须是 32 字节的标准 base64。`HTTP_ADDR` 可留空，默认 `:8080`。含特殊字符的值请用双引号包裹。

需要访问外网时设置环境变量 `HTTPS_PROXY`，不要再写 `PROXY_HOST`。

## 建表

`./cli serve` 启动时会检查数据表。缺少的表按 `sql/schema.sql` 自动创建。已有表会按同一脚本补齐列、索引、外键，并修改已有列的类型、是否可空和默认值。脚本里没有的列、索引、外键会保留。文件里的表名不带前缀。若 `MYSQL_PREFIX` 非空，表名和外键引用的表名加上同一个前缀，索引名和约束名不变。

## 启动

```
make build-cli-mac
./cli serve
```

浏览器打开 `http://127.0.0.1:8080`，使用 `.env` 中的管理员账号登录。

## Docker

本机构建并加载两个架构的镜像，不推远程仓库：

```
make docker-build
```

只构建其中一个架构时使用 `make docker-build-amd64` 或 `make docker-build-arm64`。默认镜像名是 `dockerimgsync`，tag 是 `latest`，产物为 `dockerimgsync:latest-amd64` 和 `dockerimgsync:latest-arm64`。可用 `IMAGE`、`TAG` 覆盖。

`.env` 不打进镜像，运行时挂到二进制同目录。MySQL 仍在容器外。

```
docker run --rm -p 8080:8080 -v "$(pwd)/.env:/app/.env:ro" dockerimgsync:latest-amd64
```

一起启动 MySQL 和本服务：

```
docker compose up --build
```

`compose.yaml` 会按当前机器架构构建 `dockerimgsync:latest`，并把仓库根目录的 `.env` 挂到 `/app/.env`。`MYSQL_DSN` 的主机名写 `mysql`，账号、密码、库名与 `MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE` 相同。MySQL 只在 Compose 网络内访问，数据放在卷 `mysql-data`。浏览器打开 `http://127.0.0.1:8080`。

## 使用顺序

1. 先创建登录信息（源仓库或目标仓库的账号）。
2. 再创建目标仓库。目标仓库必须事先在阿里云控制台建好，本程序不会创建仓库。
3. 最后创建同步任务。

公开镜像可以不选源登录信息；私有源从已保存的登录信息里选择。检查间隔为 0 表示只手动同步。

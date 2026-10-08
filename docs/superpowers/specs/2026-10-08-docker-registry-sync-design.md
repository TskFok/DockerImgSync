# Docker 镜像仓库同步

## 目标

把当前「查 Docker Hub 更新 → 创建 GitHub Issue → 由 hub-mirror 同步」的命令行，改成一个带登录的 Web 系统。系统用仓库协议把镜像从源仓库复制到目标仓库（例如阿里云容器镜像服务），不依赖本机 Docker、skopeo、GitHub 或 Redis。

只在源 manifest 的 digest 发生变化时复制。多架构镜像按索引原样复制，不拆成单一平台。

## 已确认的决定

- 单个 Go 进程同时提供页面、同步和定时检查。
- 复制使用 `github.com/google/go-containerregistry`，读取并写回 manifest descriptor，保留多架构索引。
- 保留 MySQL。去掉 GitHub Issue、Redis、Docker Hub 专用登录接口。
- 登录信息和目标仓库分开保存。目标仓库选择一套登录信息。同步任务只选择目标仓库。
- 源镜像必填。公开镜像可以不选源登录信息；私有源从已保存的登录信息里选择。
- 目标仓库名和 tag 可以改，留空则沿用源镜像。地址和命名空间来自所选目标仓库。
- 支持立即同步，也支持按任务设置的间隔自动检查。
- Web 只有一个管理员，用户名和密码放在 `.env`。

## 架构

`serve` 子命令启动进程。`bootstrap.Init` 仍先读二进制文件同目录的 `.env`，再连接 MySQL，然后启动 HTTP 服务和调度器。不再初始化 Redis。

四块职责：

1. **Web**：`net/http` 服务端渲染。路由使用 `github.com/go-chi/chi/v5`。未登录的请求重定向到登录页。
2. **同步引擎**：用 `remote.Head` 取源 descriptor 的 digest。需要复制时调用 `crane.Copy`（或同等的 puller.Get + pusher.Push），由库上传 blob 和 manifest。只 PUT manifest、不上传层，不算完成复制。认证只在选了登录信息时附加。Docker Hub 的 token 交换交给这个库，不再调用 `/v2/users/login`。
3. **调度器**：与 Web 同进程。每 15 秒查出到期任务并交给同步引擎。
4. **手动同步**：页面触发同一套引擎，不另写复制逻辑。

出站 HTTP 使用 `http.DefaultTransport`，因此遵循环境变量 `HTTPS_PROXY`。程序不再读取 `PROXY_HOST`。

同时进行的同步最多 2 个，用进程内信号量限制。同一任务不会并行执行。

## 配置

`.env` 仍放在打包后的二进制旁边。`.env.example` 只保留空值。

保留：

- `MYSQL_DSN`
- `MYSQL_PREFIX`（GORM 表前缀，行为与现在相同）

新增，缺失或非法时拒绝启动：

- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`
- `SESSION_SECRET`：至少 32 个字符，用于签名会话 cookie
- `CREDENTIAL_KEY`：标准 base64，解码后必须是 32 字节，用作 AES-256-GCM 密钥

新增，有默认值：

- `HTTP_ADDR`：默认 `:8080`

删除：`DOCKER_HOST`、`DOCKER_USERNAME`、`DOCKER_PASSWORD`、`GITHUB_HOST`、`GITHUB_TOKEN`、`PROXY_HOST`、`REDIS_HOST`、`REDIS_USER`、`REDIS_PASSWORD`。

## 数据模型

去掉表 `docker_image` 和 `issue`。新建表写在 `sql/schema.sql`，文件里的表名不带前缀。GORM 使用现有的 `SingularTable`，表名是单数；若设置了 `MYSQL_PREFIX`，建表时使用同一个前缀。登录信息和目标仓库的外键使用 `ON DELETE RESTRICT`。`sync_log.sync_task_id` 使用 `ON DELETE CASCADE`，这样删掉已结束的任务时一并去掉它的记录。应用层在删除前给出中文错误，不依赖数据库报错文案。

### credential

| 列 | 类型 | 说明 |
|---|---|---|
| id | int PK | |
| name | varchar(255) unique | 显示名称 |
| username | varchar(255) | |
| password_encrypted | text | AES-256-GCM 密文，base64。12 字节随机 nonce 放在密文前面 |
| created_at / updated_at | datetime | |

登录信息不保存仓库地址。

### registry

只表示目标仓库。

| 列 | 类型 | 说明 |
|---|---|---|
| id | int PK | |
| name | varchar(255) unique | 显示名称 |
| address | varchar(255) | 主机，可带端口。不含协议和路径，例如 `registry.cn-hangzhou.aliyuncs.com` |
| namespace | varchar(255) | 单段路径，不含 `/` |
| credential_id | int FK | 必填，引用 credential |
| created_at / updated_at | datetime | |

保存目标仓库时，地址不能带协议或路径，命名空间不能含 `/`。不符合则表单返回错误。

### sync_task

| 列 | 类型 | 说明 |
|---|---|---|
| id | int PK | |
| name | varchar(255) | 显示名称 |
| source_image | varchar(512) | 必须包含 tag，例如 `docker.io/library/nginx:latest` |
| source_credential_id | int FK nullable | 空表示匿名拉取 |
| registry_id | int FK | 必填 |
| dest_repository | varchar(255) | 空字符串表示使用源镜像路径的最后一段 |
| dest_tag | varchar(255) | 空字符串表示使用源 tag |
| interval_seconds | int | `0` 表示只手动同步。大于 0 时最小 60 |
| enabled | tinyint | 关闭后定时检查跳过；立即同步仍可用 |
| last_digest | varchar(255) | 上次成功复制的源 descriptor digest。失败时不改 |
| last_status | varchar(32) | `idle`、`running`、`success`、`skipped`、`failed` |
| last_error | text | 最近一次失败原因。成功或无变化时清空 |
| last_synced_at | datetime nullable | 最近一次结束时间，含无变化 |
| next_run_at | datetime nullable | 间隔为 0 时为空 |
| created_at / updated_at | datetime | |

新建或重新启用一个间隔大于 0 的任务时，`next_run_at` 设为当前时间，下一次调度就会检查。编辑任务时：间隔从 0 改为大于 0，同样把 `next_run_at` 设为当前时间；改为 0 则把 `next_run_at` 清空。间隔不是 0 时必须大于等于 60，否则表单返回错误。

### sync_log

| 列 | 类型 | 说明 |
|---|---|---|
| id | int PK | |
| sync_task_id | int FK | 任务删除时级联删除记录 |
| trigger | varchar(16) | `manual`、`schedule` 或 `startup` |
| status | varchar(16) | `success`、`skipped`、`failed` |
| source_digest | varchar(255) | 本次读到的 digest，读失败时可空 |
| message | text | 给人看的说明 |
| started_at / finished_at | datetime | |

任务列表显示 `last_status`。详情页按时间倒序显示该任务的同步记录。

### 删除规则

- 登录信息被任一目标仓库或同步任务引用时不能删除。
- 目标仓库被任一同步任务引用时不能删除。
- `last_status = running` 的任务不能删除。

编辑登录信息时，密码留空表示保持原密码。

## 页面

页面为中文。登录后进入同步任务列表。导航有三项：登录信息、目标仓库、同步任务。

| 路径 | 行为 |
|---|---|
| GET/POST `/login` | 登录 |
| POST `/logout` | 退出 |
| GET `/credentials` | 列表 |
| GET `/credentials/new`、POST `/credentials` | 新建 |
| GET `/credentials/{id}/edit`、POST `/credentials/{id}` | 编辑 |
| POST `/credentials/{id}/delete` | 删除 |
| GET `/registries` | 列表 |
| GET `/registries/new`、POST `/registries` | 新建 |
| GET `/registries/{id}/edit`、POST `/registries/{id}` | 编辑 |
| POST `/registries/{id}/delete` | 删除 |
| GET `/tasks` | 列表 |
| GET `/tasks/new`、POST `/tasks` | 新建 |
| GET `/tasks/{id}` | 详情和同步记录 |
| GET `/tasks/{id}/edit`、POST `/tasks/{id}` | 编辑 |
| POST `/tasks/{id}/delete` | 删除 |
| POST `/tasks/{id}/sync` | 立即同步 |
| POST `/tasks/{id}/toggle` | 启用或关闭 |

所有 POST 表单带 CSRF 令牌。令牌放在会话里，提交时校验。校验失败返回 400。

会话 cookie：HttpOnly、SameSite=Lax、HMAC-SHA256 签名、有效期 24 小时。管理员密码先做 SHA-256，再用 `subtle.ConstantTimeCompare` 比较，避免按明文长度提前返回。

目标仓库名和 tag 的输入框留空时，旁边说明「与源相同」。列表里的失败任务能看到 `last_error` 的摘要。

## 镜像地址

保存任务时用 `go-containerregistry` 的 reference 解析源镜像。不带 registry 的名称按 Docker Hub 补全，例如 `nginx:latest` 成为 `docker.io/library/nginx:latest`。不接受只有 digest、没有 tag 的引用。

默认目标仓库名是源镜像仓库路径的最后一段。`docker.io/library/nginx:latest` 的默认仓库名是 `nginx`，不是 `library/nginx`。路径更深时同样只取最后一段，例如 `ghcr.io/org/team/app:1` 默认为 `app`。需要保留中间路径时，在任务里填写目标仓库名。

最终目标引用：

```text
{registry.address}/{registry.namespace}/{仓库名}:{tag}
```

例子：源 `docker.io/library/nginx:latest`，目标地址 `registry.cn-hangzhou.aliyuncs.com`，命名空间 `myns`，结果是 `registry.cn-hangzhou.aliyuncs.com/myns/nginx:latest`。

## 同步流程

手动和定时共用一个引擎。立即同步的 HTTP 请求只负责启动，不在请求里等待复制结束。

1. 进程内按任务 id 加锁。锁内若该任务已是 `running`，手动同步直接提示正在同步，不启动第二次。
2. 用一条 `WHERE id IN (...)` 把即将运行的任务标为 `running`。调度器不挑选 `running` 任务。
3. 解析源引用和目标引用。源登录信息为空则匿名；否则解密该登录信息。目标始终使用目标仓库绑定的登录信息。
4. `remote.Head` 读取源 descriptor 的 digest，不下载层。这个 digest 对应整份 manifest，多架构时是索引的 digest。
5. digest 与 `last_digest` 相同：写入 `skipped` 记录，状态改为 `skipped`，清空 `last_error`，更新 `last_synced_at`，不复制。
6. digest 不同：调用 `crane.Copy`（或同等的 puller/pusher 路径）复制到目标。成功后把 `last_digest` 更新为新 digest，状态改为 `success`，清空 `last_error`，更新 `last_synced_at`，并写 `success` 记录。
7. 间隔大于 0 时，`next_run_at = 结束时间 + interval_seconds`。间隔为 0 时 `next_run_at` 保持为空。

没有「强制重推」。digest 相同就跳过。

调度器每次只用一条查询取出到期任务，并 JOIN 目标仓库和两套登录信息（源、目标）。条件：`enabled = 1` 且 `interval_seconds > 0` 且 `next_run_at <= now` 且 `last_status <> running`。不在循环里查库。选中的任务批量改为 `running`，然后各自等待信号量。每个任务结束后只更新自己的那一行，并插入自己的 `sync_log`。

## 失败处理

- 认证失败、网络错误、目标仓库不存在：`sync_log.status = failed`，`last_status = failed`，写入 `last_error`。保留原来的 `last_digest`。下次运行时间仍按间隔计算，不立刻重试。
- 阿里云容器镜像服务中的仓库需要事先在控制台创建。程序不创建仓库。
- 进程启动时，把残留的 `running` 任务改成 `failed`，`last_error` 为「进程重启，同步中断」，并补一条 `trigger = startup` 的失败同步记录。
- 页面、日志和错误文本不包含密码、密钥或 Authorization 头。

## 代码布局

沿用现有目录，替换旧的同步链路。

- `cmd/serve.go`：`serve` 命令。删除 `cmd/syncTask.go`。
- `service/sync`：引用解析、引擎接口、调度器。
- `service/web`：处理函数和 `html/template`。
- `app/model`：四张新表的模型。删除 `dockerImage.go` 和 `issue.go`。
- `utils/crypto`：加密和解密。
- `sql/schema.sql`：建表语句。

删除 `service/DockerApi`、`service/GithubApi`、`utils/cache`，以及 `global` 里的 Redis、GitHub、Docker Hub 字段。`bootstrap` 不再连接 Redis。根命令说明改为镜像同步，启动方式是 `./cli serve`。`README.md` 改为本系统的配置、建表和页面用法，去掉 hub-mirror 与 GitHub Issue 步骤。

引擎接口便于测试替换，单元测试不访问真实仓库：

```go
type Auth struct {
    Username string
    Password string
}

type Engine interface {
    Digest(ctx context.Context, ref string, auth *Auth) (string, error)
    Copy(ctx context.Context, src, dst string, srcAuth, dstAuth *Auth) error
}
```

`auth == nil` 表示匿名。

## 测试

同步引擎在测试中使用假实现。覆盖：

- 源引用解析，以及默认和自定义的目标地址拼接
- 拒绝没有 tag 的源引用
- digest 相同则跳过且不调用 Copy；不同才调用 Copy
- 源登录信息为空时 Digest 和 Copy 收到的源认证为 nil；目标认证始终存在
- 密文可解密，且不等于原文；错误密钥无法解密
- 到期判断做成纯函数：启用、间隔大于 0、`next_run_at` 已到、且不是 `running` 才到期。间隔为 0 和已禁用的任务不到期。此测试不连接 MySQL
- 启动时把 `running` 重置为 `failed`
- 登录信息或目标仓库仍被引用时删除被拒绝；运行中的任务删除被拒绝
- 管理员登录成功与失败；未登录访问页面会跳转登录
- `LoadConfig` 读取新的环境变量，并在必填项缺失时返回错误

现有 `utils/conf/conf_test.go` 改为新的配置项。测试夹具使用明显的假值，不写入真实密钥。

## 不在本次范围

- 多用户和权限
- 强制重推
- 自动在目标仓库创建命名空间或仓库
- 同步某个仓库的全部 tag
- 独立前端或独立 worker 进程
- Webhook 触发

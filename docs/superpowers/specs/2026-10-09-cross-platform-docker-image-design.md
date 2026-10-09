# 跨平台 Docker 镜像构建

## 目标

在本机构建 `linux/amd64` 与 `linux/arm64` 两个镜像，分别打 tag 并加载到本地 Docker，不推远程仓库。

## 镜像

多阶段构建。编译阶段使用 `golang:1.25-alpine`，运行在构建机架构上，按 `TARGETOS` / `TARGETARCH` 交叉编译静态二进制（`CGO_ENABLED=0`），产物路径与现有 CLI 构建一致。运行阶段使用带 CA 证书的 `alpine:3.21`。二进制位于 `/app/cli`，入口为 `cli serve`，监听 `8080`。

`.env` 不打进镜像。运行时挂到二进制同目录：

```bash
docker run --rm -p 8080:8080 -v "$(pwd)/.env:/app/.env:ro" dockerimgsync:latest-amd64
```

MySQL 保持在容器外。

## Makefile

| 目标 | 作用 |
|---|---|
| `docker-build-amd64` | `linux/amd64`，tag 为 `$(IMAGE):$(TAG)-amd64`，`--load` 进本地 |
| `docker-build-arm64` | `linux/arm64`，tag 为 `$(IMAGE):$(TAG)-arm64` |
| `docker-build` | 依次构建上面两个 |

默认 `IMAGE=dockerimgsync`、`TAG=latest`。未安装 Docker 或 buildx 时失败并说明原因。`.dockerignore` 排除 `.git`、`.env` 和本地 `cli` 二进制。

## 测试

单元测试不依赖 Docker 守护进程，检查 Dockerfile、Makefile 与 `.dockerignore` 是否包含上述约定。

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH
ARG GOPROXY=https://goproxy.cn,direct

ENV CGO_ENABLED=0 \
    GO111MODULE=on \
    GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /out/cli -ldflags "-w -s" -trimpath ./bin/cli/main.go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=build /out/cli /app/cli

EXPOSE 8080
ENTRYPOINT ["/app/cli", "serve"]

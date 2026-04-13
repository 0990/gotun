FROM golang:1.26.1 AS builder
ARG VERSION=dev
WORKDIR /src/gotun
COPY . .

RUN go env -w GOPROXY="https://goproxy.cn,direct"
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/0990/gotun.Version=${VERSION}" -o /bin/gotun ./cmd/main.go

FROM alpine:3.22 AS gotun
RUN apk add --no-cache ca-certificates mtr

WORKDIR /0990/bin
COPY --from=builder /bin/gotun .

WORKDIR /0990
CMD ["bin/gotun","-config","config/app.yaml","-tun_dir","config/tunnel"]

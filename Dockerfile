FROM golang:1.26.1 AS builder
ARG VERSION=dev
COPY . gotun
WORKDIR gotun

RUN go env -w GOPROXY="https://goproxy.cn,direct"
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/0990/gotun.Version=${VERSION}" -o /bin/gotun ./cmd/main.go

FROM scratch as gotun
WORKDIR /0990
WORKDIR bin
COPY --from=builder /bin/gotun .
WORKDIR /0990
CMD ["bin/gotun","-config","config/app.yaml","-tun_dir","config/tunnel"]

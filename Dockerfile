FROM golang:1.20.5 AS builder
COPY . gotun
WORKDIR gotun

RUN go env -w GOPROXY="https://goproxy.cn,direct"
RUN CGO_ENABLED=0 go build -o /bin/gotun ./cmd/main.go

FROM scratch as gotun
WORKDIR /0990
WORKDIR bin
COPY --from=builder /bin/gotun .
WORKDIR /0990
CMD ["bin/gotun","-config","config/app.yaml","-tun_dir","config/tunnel"]

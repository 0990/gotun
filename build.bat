::build
SET GOOS=linux
SET GOARCH=amd64
go build -o bin/gotun cmd/main.go

SET GOOS=windows
SET GOARCH=amd64
go build -o bin/gotun.exe cmd/main.go
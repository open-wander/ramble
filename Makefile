build:
	go build -o bin/rmbl-server main.go

compile:
	echo "Compiling for every OS and Platform"
	GOOS=darwin GOARCH=amd64 go build -o bin/rmbl-server-darwin main.go
	GOOS=linux GOARCH=amd64 go build -o bin/rmbl-server-linux-amd64 main.go
	# GOOS=linux GOARCH=arm64 go build -o bin/rmbl-server-linux-arm64 main.go
	GOOS=windows GOARCH=amd64 go build -o bin/rmbl-server-windows-amd64.exe main.go

binaries:
	echo "Making binaries with go releaser"
	# Brew install goreleaser
	goreleaser --snapshot --skip-publish --rm-dist

code_vul_scan:
	time gosec ./...

run:
	go run main.go
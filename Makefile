.PHONY: deps test server client client-win all clean

deps:
	go mod tidy

test: deps
	go test ./...

server: deps
	go build -o bin/gachi-server ./cmd/server

server-win: deps
	GOOS=windows GOARCH=amd64 go build -o bin/gachi-server.exe ./cmd/server

client-win: deps
	GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o bin/gachi-client.exe ./cmd/client

all: server server-win client-win

clean:
	rm -rf bin

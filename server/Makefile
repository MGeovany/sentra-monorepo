.PHONY: run test build health me kill-port

PORT ?= 8080

run:
	PORT=$(PORT) go run .

test:
	go test ./...

build:
	go build -o bin/server .

health:
	curl -i http://localhost:$(PORT)/health

# Usage: make me TOKEN="<jwt>"
me:
	curl -i -H "Authorization: Bearer $(TOKEN)" http://localhost:$(PORT)/users/me

kill-port:
	kill -9 $(shell lsof -t -i:$(PORT))

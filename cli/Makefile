.PHONY: scan add commit status log push run build

scan:
	go run ./cmd/sentra scan

add:
	go run ./cmd/sentra add .

# `make commit -m 'message'` is not a real thing in Make.
# Use `m=` / `MSG=` or call the CLI directly.
commit:
	@if [ -z "$(m)" ] && [ -z "$(MSG)" ]; then \
		echo "usage: make commit m='message' (or MSG='message')"; \
		exit 1; \
	fi
	go run ./cmd/sentra commit -m "$(or $(m),$(MSG))"

status:
	go run ./cmd/sentra status

log:
	go run ./cmd/sentra log

push:
	go run ./cmd/sentra push

# Pass raw args to Sentra, e.g.
# make run ARGS="commit -m 'new env'"
run:
	go run ./cmd/sentra $(ARGS)

build:
	go build -o bin/sentra ./cmd/sentra

login:
	go run ./cmd/sentra login

who:
	go run ./cmd/sentra who

projects:
	go run ./cmd/sentra projects


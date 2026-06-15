APP := ytdl-pro
CMD := ./cmd/ytdl-pro
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
GO ?= go
ARGS ?=

.PHONY: all build run test fmt tidy install clean help

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD)

run:
	$(GO) run $(CMD) $(ARGS)

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

install:
	$(GO) install $(CMD)

clean:
	$(GO) clean
	rm -rf $(BIN_DIR)

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make build                 Build ./bin/ytdl-pro' \
		'  make run ARGS="..."        Run the application with arguments' \
		'  make test                  Run all tests' \
		'  make fmt                   Format Go source files' \
		'  make tidy                  Update Go module dependencies' \
		'  make install               Install ytdl-pro into GOBIN/GOPATH/bin' \
		'  make clean                 Remove build output'

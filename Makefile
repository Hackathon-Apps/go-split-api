.PHONY: build
build:
	@echo ">> building app..."
	/usr/local/go/bin/go build -o go-split-api -v ./cmd/split

.DEFAULT_GOAL := build

.PHONY: build
build:
	@echo ">> building app..."
	go build -o go-split-api -v ./cmd/split

.DEFAULT_GOAL := build

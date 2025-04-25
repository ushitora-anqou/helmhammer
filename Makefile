.PHONY: test
test:
	go test ./...

.PHONY: build
build:
	go build main.go

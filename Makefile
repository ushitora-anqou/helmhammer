.PHONY: test
test:
	jsonnet jsonnet/prologue.jsonnet >/dev/null
	go test ./...

.PHONY: fmt
fmt:
	gofmt -w .
	jsonnetfmt -i jsonnet/prologue.jsonnet

.PHONY: build
build:
	go build main.go

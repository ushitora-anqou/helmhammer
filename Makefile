.PHONY: test
test: prepare-test
	jsonnet jsonnet/prologue.jsonnet >/dev/null
	go test ./...

.PHONY: fmt
fmt:
	gofmt -w .
	jsonnetfmt -i jsonnet/prologue.jsonnet

.PHONY: build
build:
	go build main.go

TESTDATA_THIRDPARTY=compiler/testdata/thirdparty

.PHONY: prepare-test
prepare-test:
	mkdir -p $(TESTDATA_THIRDPARTY)
	$(MAKE) $(TESTDATA_THIRDPARTY)/topolvm-15.5.4

$(TESTDATA_THIRDPARTY)/topolvm-15.5.4:
	cd $(TESTDATA_THIRDPARTY) ; \
	wget https://github.com/topolvm/topolvm/releases/download/topolvm-chart-v15.5.4/topolvm-15.5.4.tgz ; \
	tar xf topolvm-15.5.4.tgz ; \
	mv topolvm topolvm-15.5.4 ; \
	rm topolvm-15.5.4.tgz

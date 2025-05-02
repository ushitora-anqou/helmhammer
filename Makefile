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

.PHONY: generate-test-expected
generate-test-expected:
	cd compiler/testdata; \
		helm template topolvm thirdparty/topolvm-15.5.4 --namespace topolvm-system --values topolvm-15.5.4-1.values.yaml | yq ea -o=json '[.]' | jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | .[] | select(. != null)' | jq -s > topolvm-15.5.4-1.expected

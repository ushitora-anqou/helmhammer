TESTDATA=compiler/testdata
TESTDATA_THIRDPARTY=$(TESTDATA)/thirdparty

.PHONY: test
test: prepare-test
	jsonnet jsonnet/prologue.jsonnet >/dev/null
	go test ./...

.PHONY: fmt
fmt:
	alejandra .
	gofmt -w .
	jsonnetfmt -i jsonnet/prologue.jsonnet

.PHONY: build
build:
	go build main.go

.PHONY: prepare-test
prepare-test:
	mkdir -p $(TESTDATA_THIRDPARTY)
	$(MAKE) download-all-charts
	$(MAKE) generate-all-expected-files

define download-chart
$$(TESTDATA_THIRDPARTY)/$(1):
	cd $$(TESTDATA_THIRDPARTY) ; \
	wget $(2) ; \
	tar xf $(1).tgz ; \
	mv $(3) $(1) ; \
	rm $(1).tgz
endef

$(eval $(call download-chart,topolvm-15.5.4,https://github.com/topolvm/topolvm/releases/download/topolvm-chart-v15.5.4/topolvm-15.5.4.tgz,topolvm))

.PHONY: download-all-charts
download-all-charts: \
	$(TESTDATA_THIRDPARTY)/topolvm-15.5.4

define generate-expected-file
$$(TESTDATA)/$(1):
	cd $$(TESTDATA); $(2) | yq ea -o=json '[.]' | jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | .[] | select(. != null)' | jq -s > $(1)
endef

$(eval $(call generate-expected-file,hello.expected, \
	helm template hello hello \
))
$(eval $(call generate-expected-file,topolvm-15.5.4-0.expected, \
	helm template topolvm thirdparty/topolvm-15.5.4 \
))
$(eval $(call generate-expected-file,topolvm-15.5.4-1.expected, \
	helm template topolvm thirdparty/topolvm-15.5.4 --include-crds --namespace topolvm-system --values topolvm-15.5.4-1.values.yaml \
))

.PHONY: generate-all-expected-files
generate-all-expected-files: \
	$(TESTDATA)/hello.expected \
	$(TESTDATA)/topolvm-15.5.4-0.expected \
	$(TESTDATA)/topolvm-15.5.4-1.expected

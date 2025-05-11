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
$(eval $(call download-chart,reloader-2.1.3,https://stakater.github.io/stakater-charts/reloader-2.1.3.tgz,reloader))
$(eval $(call download-chart,cloudflare-tunnel-ingress-controller-0.0.18,https://helm.strrl.dev/cloudflare-tunnel-ingress-controller-0.0.18.tgz,cloudflare-tunnel-ingress-controller))
$(eval $(call download-chart,sidekiq-prometheus-exporter-0.2.1,https://github.com/Strech/sidekiq-prometheus-exporter/releases/download/v0.2.0-4/sidekiq-prometheus-exporter-0.2.1.tgz,sidekiq-prometheus-exporter))
$(eval $(call download-chart,cert-manager-v1.17.2,https://charts.jetstack.io/charts/cert-manager-v1.17.2.tgz,cert-manager))
$(eval $(call download-chart,argo-cd-7.9.0,https://github.com/argoproj/argo-helm/releases/download/argo-cd-7.9.0/argo-cd-7.9.0.tgz,argo-cd))

.PHONY: download-all-charts
download-all-charts: \
	$(TESTDATA_THIRDPARTY)/topolvm-15.5.4 \
	$(TESTDATA_THIRDPARTY)/reloader-2.1.3 \
	$(TESTDATA_THIRDPARTY)/cloudflare-tunnel-ingress-controller-0.0.18 \
	$(TESTDATA_THIRDPARTY)/sidekiq-prometheus-exporter-0.2.1 \
	$(TESTDATA_THIRDPARTY)/cert-manager-v1.17.2 \
	$(TESTDATA_THIRDPARTY)/argo-cd-7.9.0

define generate-expected-file
$$(TESTDATA)/$(1):
	cd $$(TESTDATA); $(2) | yq ea -o=json '[.]' | jq 'sort_by([.apiVersion, .kind, .metadata.namespace, .metadata.name]) | .[] | select(. != null)' | jq -s > $(1)
endef

$(eval $(call generate-expected-file,skeleton.expected, \
	helm template skeleton skeleton \
))
$(eval $(call generate-expected-file,testchart.expected, \
	helm template --kube-version="v1.32.0" testchart testchart \
))
$(eval $(call generate-expected-file,topolvm-15.5.4-0.expected, \
	helm template topolvm thirdparty/topolvm-15.5.4 \
))
$(eval $(call generate-expected-file,topolvm-15.5.4-1.expected, \
	helm template topolvm thirdparty/topolvm-15.5.4 --include-crds --namespace topolvm-system --values topolvm-15.5.4-1.values.yaml \
))
$(eval $(call generate-expected-file,reloader-2.1.3-0.expected, \
	helm template reloader thirdparty/reloader-2.1.3 \
))
$(eval $(call generate-expected-file,reloader-2.1.3-1.expected, \
	helm template reloader thirdparty/reloader-2.1.3 --include-crds --namespace reloader --values reloader-2.1.3-1.values.yaml \
))
$(eval $(call generate-expected-file,cloudflare-tunnel-ingress-controller-0.0.18-0.expected, \
	helm template cloudflare-tunnel-ingress-controller thirdparty/cloudflare-tunnel-ingress-controller-0.0.18 \
))
$(eval $(call generate-expected-file,cloudflare-tunnel-ingress-controller-0.0.18-1.expected, \
	helm template cloudflare-tunnel-ingress-controller thirdparty/cloudflare-tunnel-ingress-controller-0.0.18 --include-crds --namespace ctic --values cloudflare-tunnel-ingress-controller-0.0.18-1.values.yaml \
))
$(eval $(call generate-expected-file,sidekiq-prometheus-exporter-0.2.1-0.expected, \
	helm template sidekiq-prometheus-exporter thirdparty/sidekiq-prometheus-exporter-0.2.1 \
))
$(eval $(call generate-expected-file,sidekiq-prometheus-exporter-0.2.1-1.expected, \
	helm template sidekiq-prometheus-exporter thirdparty/sidekiq-prometheus-exporter-0.2.1 \
		--include-crds --namespace sidekiq-prometheus-exporter \
		--values sidekiq-prometheus-exporter-0.2.1-1.values.yaml \
))
$(eval $(call generate-expected-file,cert-manager-v1.17.2-0.expected, \
	helm template cert-manager thirdparty/cert-manager-v1.17.2 \
))
$(eval $(call generate-expected-file,cert-manager-v1.17.2-1.expected, \
	helm template cert-manager thirdparty/cert-manager-v1.17.2 \
		--include-crds --namespace cert-manager \
		--values cert-manager-v1.17.2-1.values.yaml \
))
$(eval $(call generate-expected-file,argo-cd-7.9.0-0.expected, \
	helm template argo-cd thirdparty/argo-cd-7.9.0 \
))
$(eval $(call generate-expected-file,argo-cd-7.9.0-1.expected, \
	helm template argo-cd thirdparty/argo-cd-7.9.0 \
		--include-crds --namespace argocd \
		--values argo-cd-7.9.0-1.values.yaml \
))

.PHONY: generate-all-expected-files
generate-all-expected-files: \
	$(TESTDATA)/skeleton.expected \
	$(TESTDATA)/testchart.expected \
	$(TESTDATA)/topolvm-15.5.4-0.expected \
	$(TESTDATA)/topolvm-15.5.4-1.expected \
	$(TESTDATA)/reloader-2.1.3-0.expected \
	$(TESTDATA)/reloader-2.1.3-1.expected \
	$(TESTDATA)/cloudflare-tunnel-ingress-controller-0.0.18-0.expected \
	$(TESTDATA)/cloudflare-tunnel-ingress-controller-0.0.18-1.expected \
	$(TESTDATA)/sidekiq-prometheus-exporter-0.2.1-0.expected \
	$(TESTDATA)/sidekiq-prometheus-exporter-0.2.1-1.expected \
	$(TESTDATA)/cert-manager-v1.17.2-0.expected \
	$(TESTDATA)/cert-manager-v1.17.2-1.expected \
	$(TESTDATA)/argo-cd-7.9.0-0.expected \
	$(TESTDATA)/argo-cd-7.9.0-1.expected

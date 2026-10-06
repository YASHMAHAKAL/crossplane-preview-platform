.PHONY: verify evaluator-test function-test app-test render-namespace render-vcluster
GO_BIN ?= go
CROSSPLANE_BIN ?= crossplane
verify: evaluator-test function-test app-test

evaluator-test:
	cd platform/evaluator && $(GO_BIN) test ./...

function-test:
	cd platform/crossplane/function && GOFLAGS=-tags=http2legacy $(GO_BIN) test ./...

app-test:
	cd app/incident-tracker && npm test && node --check public/app.js

render-namespace:
	$(CROSSPLANE_BIN) composition render tests/render/namespace-xr.json platform/crossplane/compositions/namespace.yaml tests/render/functions.yaml --crossplane-version=v2.4.0 --xrd=platform/crossplane/xrd.yaml

render-vcluster:
	$(CROSSPLANE_BIN) composition render tests/render/vcluster-xr.json platform/crossplane/compositions/vcluster.yaml tests/render/functions.yaml --crossplane-version=v2.4.0 --xrd=platform/crossplane/xrd.yaml

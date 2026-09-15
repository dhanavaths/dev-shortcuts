rootMKfilePath:=$(abspath $(lastword $(MAKEFILE_LIST)))
rootMKfileDir:=$(shell cd $(shell dirname $(rootMKfilePath)); pwd)

GOHOSTOS ?= $(shell go env GOHOSTOS)
GOHOSTARCH ?= $(shell go env GOHOSTARCH)
CODE_DIR="${rootMKfileDir}/.."
TAG := $(shell git -C ${CODE_DIR} describe --tags --always)
VERSION :=
DEPLOYMENT_IMAGE_VERSION ?= v107
CLUSTER_CONTEXT ?= dev-control-cluster
SKIP_KIND_LOAD_IMAGE ?= true
DEPLOYMENT_TIMESTAMP := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
DEPLOYMENT_APP_VERSION := $(shell date -u +"%Y%m%d%H%M%S")
# Adds a '-dirty' suffix to version string if there are uncommitted changes
changes := $(shell git -C ${CODE_DIR} status --porcelain)
ifeq ($(changes),)
	VERSION := $(TAG)
else
	VERSION := $(TAG)-dirty
endif

LDFLAGS := "-X cmd.version=$(VERSION) -extldflags '-static'"


# core paths
outputDir:=$(rootMKfileDir)/_output
hackDir:=$(rootMKfileDir)/hack
cmdDir:=$(rootMKfileDir)/cmd
pkgDir:=$(rootMKfileDir)/pkg
testDir:=$(rootMKfileDir)/test
internalDir:=$(rootMKfileDir)/internal
apiImportConfigPath:=$(hackDir)/kubernetes-api-import-config.yaml

# version mgmt -- TODO: @khenidak add release version based on tags.
VERSION:=$(shell git -C ${CODE_DIR} rev-parse --short HEAD)
BUILDTIME:=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# go build flags -  for now we assume all binaries are built the same way
GOLDFLAGS+=-X main.BuildTime=$(BUILDTIME)
GOFLAGS=-ldflags "$(GOLDFLAGS)"

controllerName:=demo-app
controllerBuildDir:=$(cmdDir)/demo-app
controllerBuildFlags:=$(GOFLAGS)
wsClientDir:=$(cmdDir)/ws-client
WS_CLIENT_URL ?= wss://127.0.0.1:10001/ws
WS_CLIENT_MESSAGE ?= ping
WS_CLIENT_COUNT ?= 10
WS_CLIENT_INTERVAL ?= 1s


build-demo-app:
	@echo "$(YELLOW)** Build: $(controllerName) $(DEFAULTCOLOR)"
	@go build -o $(outputDir)/$(controllerName) $(controllerBuildFlags) $(controllerBuildDir)/.
	@echo "$(GREEN)** Done:  $(outputDir)/$(controllerName) $(DEFAULTCOLOR)"

ws-client:
	@go run $(wsClientDir)/. -url $(WS_CLIENT_URL) -message "$(WS_CLIENT_MESSAGE)" -count $(WS_CLIENT_COUNT) -interval $(WS_CLIENT_INTERVAL)

docker-demo-app:
	@echo ${CODE_DIR}
	@echo "$(YELLOW)** Docker: demo-app $(DEFAULTCOLOR)"
	@docker build -t demo-app:$(DEPLOYMENT_IMAGE_VERSION) -f $(rootMKfileDir)/Dockerfile.demo-app $(rootMKfileDir)

docker-pyapp:
	@echo "$(YELLOW)** Docker: pyapp $(DEFAULTCOLOR)"
	@docker build -t pyapp:v001 -f $(rootMKfileDir)/Dockerfile.pyapp $(rootMKfileDir)


load-demo-app-image:
	@echo -n "Kind cluster name (Enter 'cc' for current cluster): "; \
	read KIND_CLUSTER_NAME; \
	if [ "$$KIND_CLUSTER_NAME" = "cc" ]; then \
		cluster_list=$$(kubectl $$kube_context config current-context | cut -d'-' -f2-); \
	elif [ -z "$$KIND_CLUSTER_NAME" ]; then \
		cluster_list=$$(kind get clusters); \
	else \
		cluster_list=$$KIND_CLUSTER_NAME; \
	fi; \
	for cluster in $$cluster_list; do \
		echo "Loading image demo-app:$(DEPLOYMENT_IMAGE_VERSION) into cluster $$cluster..."; \
		kind load docker-image demo-app:$(DEPLOYMENT_IMAGE_VERSION) --name $$cluster; \
	done

deploy:
#	make -f ~/dev-shortcuts/demo-apps/Makefile deploy YAML=demo-app-ds-v0 DEPLOYMENT_IMAGE_VERSION=v007
#	make -f ~/dev-shortcuts/demo-apps/Makefile deploy YAML=demo-app-drdep-ds-crash-test
#	make -f ~/dev-shortcuts/demo-apps/Makefile deploy YAML=demo-app-ds
#   make -f ~/dev-shortcuts/demo-apps/Makefile deploy YAML=demo-app-rwpr
	@make -f ${rootMKfilePath} docker-demo-app DEPLOYMENT_IMAGE_VERSION=${DEPLOYMENT_IMAGE_VERSION} YAML=${YAML}
	@if [ "$(SKIP_KIND_LOAD_IMAGE)" != "true" ]; then \
		make -f ${rootMKfilePath} load-demo-app-image DEPLOYMENT_IMAGE_VERSION=${DEPLOYMENT_IMAGE_VERSION} YAML=${YAML}; \
	else \
		echo "$(YELLOW)** Skipping load-demo-app-image (SKIP_KIND_LOAD_IMAGE=true) $(DEFAULTCOLOR)"; \
	fi
	sed \
	  -e "s/DEPLOYMENT_APP_VERSION/$(DEPLOYMENT_APP_VERSION)/g" \
	  -e "s/DEPLOYMENT_IMAGE_VERSION/$(DEPLOYMENT_IMAGE_VERSION)/g" \
	  -e "s/DEPLOYMENT_TIMESTAMP/$(DEPLOYMENT_TIMESTAMP)/g" \
	  ${rootMKfileDir}/yaml/${YAML}.yaml | kubectl $$kube_context --context kind-$(CLUSTER_CONTEXT) apply -f -
deploy-yaml:
#	make -f ~/dev-shortcuts/demo-apps/Makefile deploy-yaml YAML=demo-app-ds
#   make -f ~/dev-shortcuts/demo-apps/Makefile deploy-yaml YAML=demo-app-rwpr
	sed \
	  -e "s/DEPLOYMENT_APP_VERSION/$(DEPLOYMENT_APP_VERSION)/g" \
	  -e "s/DEPLOYMENT_IMAGE_VERSION/$(DEPLOYMENT_IMAGE_VERSION)/g" \
	  -e "s/DEPLOYMENT_TIMESTAMP/$(DEPLOYMENT_TIMESTAMP)/g" \
	  ${rootMKfileDir}/yaml/${YAML}.yaml | kubectl $$kube_context --context kind-$(CLUSTER_CONTEXT) apply -f -

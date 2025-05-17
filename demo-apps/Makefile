rootMKfilePath:=$(abspath $(lastword $(MAKEFILE_LIST)))
rootMKfileDir:=$(shell cd $(shell dirname $(rootMKfilePath)); pwd)

GOHOSTOS ?= $(shell go env GOHOSTOS)
GOHOSTARCH ?= $(shell go env GOHOSTARCH)
CLUSTER_FLEET_CODE_DIR="/root/workspace/ClusterFleet"
TAG := $(shell git -C ${CLUSTER_FLEET_CODE_DIR} describe --tags --always)
VERSION :=

# Adds a '-dirty' suffix to version string if there are uncommitted changes
changes := $(shell git -C ${CLUSTER_FLEET_CODE_DIR} status --porcelain)
ifeq ($(changes),)
	VERSION := $(TAG)
else
	VERSION := $(TAG)-dirty
endif
LDFLAGS := "-X dev.azure.com/msasg/Falcon/_git/clusterfleet/cmd.version=$(VERSION) -extldflags '-static'"


# core paths
outputDir:=$(rootMKfileDir)/_output
hackDir:=$(rootMKfileDir)/hack
cmdDir:=$(rootMKfileDir)/cmd
pkgDir:=$(rootMKfileDir)/pkg
testDir:=$(rootMKfileDir)/test
internalDir:=$(rootMKfileDir)/internal
apiImportConfigPath:=$(hackDir)/kubernetes-api-import-config.yaml

# version mgmt -- TODO: @khenidak add release version based on tags.
VERSION:=$(shell git -C ${CLUSTER_FLEET_CODE_DIR} rev-parse --short HEAD)
BUILDTIME:=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# go build flags -  for now we assume all binaries are built the same way
GOLDFLAGS+=-X main.BuildTime=$(BUILDTIME)
GOFLAGS=-ldflags "$(GOLDFLAGS)"

controllerName:=demo-app
controllerBuildDir:=$(cmdDir)/demo-app
controllerBuildFlags:=$(GOFLAGS)


build-demo-app:
	@echo "$(YELLOW)** Build: $(controllerName) $(DEFAULTCOLOR)"
	@go build -o $(outputDir)/$(controllerName) $(controllerBuildFlags) $(controllerBuildDir)/.
	@echo "$(GREEN)** Done:  $(outputDir)/$(controllerName) $(DEFAULTCOLOR)"

docker-demo-app:
	@echo "$(YELLOW)** Docker: demo-app $(DEFAULTCOLOR)"
	@docker build -t demo-app:v007 -f $(rootMKfileDir)/Dockerfile.demo-app $(rootMKfileDir)

docker-pyapp:
	@echo "$(YELLOW)** Docker: pyapp $(DEFAULTCOLOR)"
	@docker build -t pyapp:v001 -f $(rootMKfileDir)/Dockerfile.pyapp $(rootMKfileDir)

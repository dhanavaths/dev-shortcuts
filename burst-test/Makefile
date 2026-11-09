# Makefile for burst-test: kube-prometheus-stack + stress-ng workload
#
# Usage:
#   make help                # list targets
#   make install             # install prom stack + set 5s scrape + deploy workload
#   make uninstall           # tear everything down

SHELL := /bin/bash

# ---- Config (override on CLI: `make install NS=poc`) ----
MON_NS          ?= monitoring
APP_NS          ?= poc
RELEASE         ?= monitor
CHART           ?= prometheus-community/kube-prometheus-stack
PROM_CR         ?= monitor-kube-prometheus-st-prometheus
GRAFANA_SVC     ?= monitor-grafana
PROM_SVC        ?= monitor-kube-prometheus-st-prometheus
SCRAPE_INTERVAL ?= 5s
GRAFANA_PASSWD  ?= admin
WORKLOAD_YAML   ?= micro-burst-test.yaml
WORKLOAD_NAME   ?= micro-burst-test

.DEFAULT_GOAL := help

## help: show this help
.PHONY: help
help:
	@echo "Targets:"
	@awk '/^## /{sub(/^## /,""); print "  " $$0}' $(MAKEFILE_LIST)

# ---------- Prometheus stack ----------

## repo-add: add and update the prometheus-community helm repo
.PHONY: repo-add
repo-add:
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	helm repo update

## install-prom: install kube-prometheus-stack into $(MON_NS)
.PHONY: install-prom
install-prom: repo-add
	helm install $(RELEASE) $(CHART) \
	  --namespace $(MON_NS) --create-namespace \
	  --set grafana.adminPassword=$(GRAFANA_PASSWD)

## upgrade-prom: helm upgrade (e.g., to change grafana password)
.PHONY: upgrade-prom
upgrade-prom:
	helm upgrade $(RELEASE) $(CHART) \
	  -n $(MON_NS) \
	  --set grafana.adminPassword=$(GRAFANA_PASSWD)

## scrape-5s: patch Prometheus CR for high-resolution polling
.PHONY: scrape-5s
scrape-5s:
	kubectl patch prometheus -n $(MON_NS) $(PROM_CR) \
	  --type=merge -p '{"spec":{"scrapeInterval":"$(SCRAPE_INTERVAL)"}}'

## uninstall-prom: remove the prometheus stack
.PHONY: uninstall-prom
uninstall-prom:
	-helm uninstall $(RELEASE) -n $(MON_NS)

# ---------- Port forwards ----------

## pf-grafana: port-forward Grafana to localhost:3000 (admin/$(GRAFANA_PASSWD))
.PHONY: pf-grafana
pf-grafana:
	@echo "Grafana: http://localhost:3000  (admin / $(GRAFANA_PASSWD))"
	kubectl port-forward svc/$(GRAFANA_SVC) -n $(MON_NS) 3000:80

## pf-prom: port-forward Prometheus to localhost:9090
.PHONY: pf-prom
pf-prom:
	@echo "Prometheus: http://localhost:9090"
	kubectl port-forward svc/$(PROM_SVC) -n $(MON_NS) 9090:9090

# ---------- Workload ----------

## ns: create the application namespace
.PHONY: ns
ns:
	-kubectl create ns $(APP_NS)

## deploy: apply the micro-burst-test workload
.PHONY: deploy
deploy: ns
	kubectl apply -f $(WORKLOAD_YAML)

## delete: delete the micro-burst-test workload
.PHONY: delete
delete:
	-kubectl -n $(APP_NS) delete deploy $(WORKLOAD_NAME)

## status: show workload pods
.PHONY: status
status:
	kubectl -n $(APP_NS) get deploy,pod -l app=burst-test -o wide

## logs: tail logs from the workload
.PHONY: logs
logs:
	kubectl -n $(APP_NS) logs -l app=burst-test -f --tail=100

# ---------- All-in-one ----------

## install: install prom stack, set 5s scrape, deploy workload
.PHONY: install
install: install-prom scrape-5s deploy

## uninstall: delete workload and prom stack
.PHONY: uninstall
uninstall: delete uninstall-prom

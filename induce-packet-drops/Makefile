IMAGE        ?= induce-packet-drops:v1
KIND_CLUSTER ?= dev-control-cluster
NS           ?= netdrop
KCTL          = kubectl --context kind-$(KIND_CLUSTER) -n $(NS)

.PHONY: build load deploy ns clean logs experiment

build:
	docker build -t $(IMAGE) .

load: build
	kind load docker-image $(IMAGE) --name $(KIND_CLUSTER)

ns:
	kubectl --context kind-$(KIND_CLUSTER) get ns $(NS) >/dev/null 2>&1 || \
	  kubectl --context kind-$(KIND_CLUSTER) create ns $(NS)

deploy: load ns
	sed "s|IMAGE_PLACEHOLDER|$(IMAGE)|g" yaml/discovery.yaml          | $(KCTL) apply -f -
	sed "s|IMAGE_PLACEHOLDER|$(IMAGE)|g" yaml/router-baseline.yaml    | $(KCTL) apply -f -
	sed "s|IMAGE_PLACEHOLDER|$(IMAGE)|g" yaml/router-proposed.yaml    | $(KCTL) apply -f -
	# Image tag is pinned (v1), so apply alone won't roll pods.
	# Force a restart so every deploy picks up the freshly-loaded image
	# and resets counters/cache state.
	$(KCTL) rollout restart deploy/service-discovery deploy/router-baseline deploy/router-proposed
	$(KCTL) rollout status  deploy/service-discovery
	$(KCTL) rollout status  deploy/router-baseline
	$(KCTL) rollout status  deploy/router-proposed

netem-on:
	sed "s|IMAGE_PLACEHOLDER|$(IMAGE)|g" yaml/netem-job.yaml | $(KCTL) apply -f -

netem-off:
	$(KCTL) delete job netem-injector --ignore-not-found

logs-baseline:
	$(KCTL) logs -f deploy/router-baseline

logs-proposed:
	$(KCTL) logs -f deploy/router-proposed

experiment:
	./scripts/run-experiment.sh

clean:
	$(KCTL) delete deploy,svc,job --all --ignore-not-found

.PHONY: ensure-local-registry start-image-syncer stop-image-syncer logs-image-syncer build-image-syncer

# Registry/syncer module defaults. Main Makefile may override any of these.
local-registry-name ?= kind-registry
local-registry-port ?= 5001
registry-cors-origin ?= http://localhost:5080
image-syncer-container-name ?= kind-image-syncer
image-syncer-image-name ?= kind-image-syncer:local
initial-sync-parallel ?= true
initial-sync-max-parallel ?= 6
registry-host ?= 127.0.0.1
registry-port ?= $(local-registry-port)
registry-name ?= $(local-registry-name)
registry-api-endpoint ?= http://$(registry-name):5000
target-repository-prefix ?=

build-image-syncer:
	docker build -t $(image-syncer-image-name) hack/sample/kind/kind-registry-syncer

ensure-local-registry:
	@if docker inspect $(local-registry-name) >/dev/null 2>&1; then \
		if ! docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' $(local-registry-name) | grep -Fxq 'REGISTRY_STORAGE_DELETE_ENABLED=true' \
			|| ! docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' $(local-registry-name) | grep -Fxq 'REGISTRY_HTTP_HEADERS_Access-Control-Allow-Origin=["$(registry-cors-origin)"]'; then \
			echo "Recreating $(local-registry-name) with delete API and CORS enabled"; \
			docker rm -f $(local-registry-name) >/dev/null; \
			docker run -d --restart=always \
				-e REGISTRY_STORAGE_DELETE_ENABLED=true \
				-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Origin='["$(registry-cors-origin)"]' \
				-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Methods='[HEAD,GET,OPTIONS,DELETE]' \
				-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Headers='[Authorization,Accept,Cache-Control]' \
				-e REGISTRY_HTTP_HEADERS_Access-Control-Expose-Headers='[Docker-Content-Digest]' \
				-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Credentials='[true]' \
				-p $(local-registry-port):5000 --name $(local-registry-name) registry:2 >/dev/null; \
		fi; \
		if [ "$$(docker inspect -f '{{.State.Running}}' $(local-registry-name))" != "true" ]; then \
			docker start $(local-registry-name) >/dev/null; \
		fi; \
	else \
		docker run -d --restart=always \
			-e REGISTRY_STORAGE_DELETE_ENABLED=true \
			-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Origin='["$(registry-cors-origin)"]' \
			-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Methods='[HEAD,GET,OPTIONS,DELETE]' \
			-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Headers='[Authorization,Accept,Cache-Control]' \
			-e REGISTRY_HTTP_HEADERS_Access-Control-Expose-Headers='[Docker-Content-Digest]' \
			-e REGISTRY_HTTP_HEADERS_Access-Control-Allow-Credentials='[true]' \
			-p $(local-registry-port):5000 --name $(local-registry-name) registry:2 >/dev/null; \
	fi; \
	if docker network inspect kind >/dev/null 2>&1; then \
		docker network connect kind $(local-registry-name) >/dev/null 2>&1 || true; \
	fi

start-image-syncer: ensure-local-registry build-image-syncer
	-docker rm -f $(image-syncer-container-name)
	docker run -d --name $(image-syncer-container-name) --restart=always \
		-v /var/run/docker.sock:/var/run/docker.sock \
		--add-host=host.docker.internal:host-gateway \
		-e REGISTRY_HOST=$(registry-host) \
		-e REGISTRY_PORT=$(registry-port) \
		-e REGISTRY_NAME=$(registry-name) \
		-e REGISTRY_API_ENDPOINT=$(registry-api-endpoint) \
		-e TARGET_REPOSITORY_PREFIX='$(target-repository-prefix)' \
		-e INITIAL_SYNC_PARALLEL=$(initial-sync-parallel) \
		-e INITIAL_SYNC_MAX_PARALLEL=$(initial-sync-max-parallel) \
		-e DELETE_BEFORE_PUSH=true \
		-e EXCLUDE_PATTERNS='kindest/node:|registry:2|localhost:|127.0.0.1:|host.docker.internal:|kind-image-syncer:' \
		$(image-syncer-image-name)
	-docker network connect kind $(image-syncer-container-name)

stop-image-syncer:
	-docker rm -f $(image-syncer-container-name)

logs-image-syncer:
	docker logs -f $(image-syncer-container-name)

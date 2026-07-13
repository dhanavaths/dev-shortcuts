#!/bin/sh

set -eu

REGISTRY_HOST="${REGISTRY_HOST:-localhost}"
REGISTRY_PORT="${REGISTRY_PORT:-5001}"
TARGET_PREFIX="${REGISTRY_HOST}:${REGISTRY_PORT}"
REGISTRY_NAME="${REGISTRY_NAME:-kind-registry}"
REGISTRY_API_ENDPOINT="${REGISTRY_API_ENDPOINT:-http://${REGISTRY_NAME}:5000}"
SYNC_EXISTING_ON_START="${SYNC_EXISTING_ON_START:-true}"
INITIAL_SYNC_PARALLEL="${INITIAL_SYNC_PARALLEL:-true}"
INITIAL_SYNC_MAX_PARALLEL="${INITIAL_SYNC_MAX_PARALLEL:-6}"
DELETE_BEFORE_PUSH="${DELETE_BEFORE_PUSH:-true}"
EXCLUDE_PATTERNS="${EXCLUDE_PATTERNS:-kindest/node:|registry:2|localhost:}"
TARGET_REPOSITORY_PREFIX="${TARGET_REPOSITORY_PREFIX:-}"

log() {
    ts="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ 2>/dev/null || true)"
    if ! printf '%s\n' "$ts" | grep -Eq '\.[0-9]{3}Z$'; then
        ts="$(date -u +%Y-%m-%dT%H:%M:%S).000Z"
    fi
    printf '[kind-image-syncer] [%s] %s\n' "$ts" "$*"
}

normalize_repo_part() {
    repo_part="$1"
    first_segment="${repo_part%%/*}"

    # Strip a registry host prefix (e.g. quay.io/, localhost:5001/) so the
    # repository path matches what containerd requests from the mirror.
    if [ "$repo_part" != "$first_segment" ]; then
        case "$first_segment" in
            *.*|*:*|localhost)
                repo_part="${repo_part#*/}"
                ;;
        esac
    fi

    # Docker Hub official images are single-segment names that resolve to
    # docker.io/library/<name>. Bare "<component>:local" images follow the same
    # rule, so store them under "library/" to be served via the docker.io mirror.
    case "$repo_part" in
        */*)
            printf '%s\n' "$repo_part"
            ;;
        *)
            printf '%s\n' "library/${repo_part}"
            ;;
    esac
}

should_skip_ref() {
    image_ref="$1"

    if [ -z "$image_ref" ] || [ "$image_ref" = "<none>:<none>" ]; then
        return 0
    fi

    case "$image_ref" in
        "${TARGET_PREFIX}"/*)
            return 0
            ;;
    esac

    old_ifs="$IFS"
    IFS='|'
    for pattern in $EXCLUDE_PATTERNS; do
        case "$image_ref" in
            *"$pattern"*)
                IFS="$old_ifs"
                return 0
                ;;
        esac
    done
    IFS="$old_ifs"

    return 1
}

push_ref() {
    source_ref="$1"

    if should_skip_ref "$source_ref"; then
        return 0
    fi

    repo_part="${source_ref%:*}"
    tag_part="${source_ref##*:}"

    # If there is no explicit tag, Docker defaults to latest.
    if [ "$repo_part" = "$source_ref" ]; then
        repo_part="$source_ref"
        tag_part="latest"
        source_ref="${repo_part}:${tag_part}"
    fi

    if ! docker image inspect "$source_ref" >/dev/null 2>&1; then
        return 0
    fi

    local_image_id="$(docker image inspect "$source_ref" --format '{{.Id}}' 2>/dev/null || true)"
    if [ -z "$local_image_id" ]; then
        return 0
    fi

    normalized_repo_part="$(normalize_repo_part "$repo_part")"

    target_repo="$normalized_repo_part"
    if [ -n "$TARGET_REPOSITORY_PREFIX" ]; then
        target_repo="${TARGET_REPOSITORY_PREFIX}/${normalized_repo_part}"
    fi

    if is_registry_in_sync "$source_ref" "$target_repo" "$tag_part"; then
        log "Already in sync, skipping ${source_ref}"
        return 0
    fi

    target_ref="${TARGET_PREFIX}/${target_repo}:${tag_part}"

    if [ "$DELETE_BEFORE_PUSH" = "true" ]; then
        delete_target_ref "$target_repo" "$tag_part"
    fi

    log "Syncing ${source_ref} -> ${target_ref}"
    docker tag "$source_ref" "$target_ref"
    if ! docker push "$target_ref"; then
        log "Push failed for ${target_ref}; will retry on the next image event"
        return 0
    fi
}

is_registry_in_sync() {
    source_ref="$1"
    repo_part="$2"
    tag_part="$3"

    local_digest="$(docker image inspect "$source_ref" --format '{{.Id}}' 2>/dev/null || true)"
    if [ -z "$local_digest" ]; then
        return 1
    fi

    local_repo_digests="$(docker image inspect "$source_ref" --format '{{range .RepoDigests}}{{println .}}{{end}}' 2>/dev/null || true)"

    registry_manifest_digest="$(curl -fsSI \
        -H 'Accept: application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json' \
        "${REGISTRY_API_ENDPOINT}/v2/${repo_part}/manifests/${tag_part}" \
        2>/dev/null \
        | tr -d '\r' \
        | awk -F': ' 'tolower($1)=="docker-content-digest" {print $2}' \
        || true)"

    registry_config_digest="$(curl -fsS \
        -H 'Accept: application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json' \
        "${REGISTRY_API_ENDPOINT}/v2/${repo_part}/manifests/${tag_part}" \
        2>/dev/null \
        | jq -r '.config.digest // empty' \
        || true)"

    if [ -n "$registry_config_digest" ] && [ "$local_digest" = "$registry_config_digest" ]; then
        return 0
    fi

    if [ -n "$registry_manifest_digest" ] && [ -n "$local_repo_digests" ]; then
        if printf '%s\n' "$local_repo_digests" | awk -F'@' -v target="$registry_manifest_digest" '$2==target {found=1} END {exit(found?0:1)}'; then
            return 0
        fi
    fi

    return 1
}

delete_target_ref() {
    repo_part="$1"
    tag_part="$2"

    digest="$(curl -fsSI \
        -H 'Accept: application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json' \
        "${REGISTRY_API_ENDPOINT}/v2/${repo_part}/manifests/${tag_part}" \
        2>/dev/null \
        | tr -d '\r' \
        | awk -F': ' 'tolower($1)=="docker-content-digest" {print $2}' \
        || true)"

    if [ -z "$digest" ]; then
        log "No existing registry manifest found for ${repo_part}:${tag_part}; skipping delete"
        return 0
    fi

    log "Deleting existing registry manifest ${repo_part}:${tag_part} (${digest})"
    curl -fsS -X DELETE "${REGISTRY_API_ENDPOINT}/v2/${repo_part}/manifests/${digest}" >/dev/null || true
}

push_all_tags_for_image_id() {
    image_id="$1"

    if [ -z "$image_id" ]; then
        return 0
    fi

    refs="$(docker image inspect "$image_id" --format '{{range .RepoTags}}{{println .}}{{end}}' 2>/dev/null || true)"
    if [ -z "$refs" ]; then
        return 0
    fi

    printf '%s\n' "$refs" | while IFS= read -r ref; do
        push_ref "$ref"
    done
}

initial_sync() {
    if [ "$INITIAL_SYNC_PARALLEL" != "true" ]; then
        log "Running initial image sync"
        docker image ls --format '{{.Repository}}:{{.Tag}}' | while IFS= read -r ref; do
            push_ref "$ref"
        done
        log "Initial image sync completed"
        return 0
    fi

    case "$INITIAL_SYNC_MAX_PARALLEL" in
        ''|*[!0-9]*)
            workers=6
            ;;
        *)
            workers="$INITIAL_SYNC_MAX_PARALLEL"
            ;;
    esac

    if [ "$workers" -le 1 ]; then
        log "Running initial image sync"
        docker image ls --format '{{.Repository}}:{{.Tag}}' | while IFS= read -r ref; do
            push_ref "$ref"
        done
        log "Initial image sync completed"
        return 0
    fi

    log "Running initial image sync in parallel (${workers} workers)"

    refs_file="$(mktemp)"
    docker image ls --format '{{.Repository}}:{{.Tag}}' > "$refs_file"

    pids=""
    running=0

    while IFS= read -r ref; do
        push_ref "$ref" &
        pid="$!"
        if [ -n "$pids" ]; then
            pids="${pids} ${pid}"
        else
            pids="$pid"
        fi
        running=$((running + 1))

        if [ "$running" -ge "$workers" ]; then
            first_pid="${pids%% *}"
            wait "$first_pid" || true
            case "$pids" in
                *' '*)
                    pids="${pids#* }"
                    ;;
                *)
                    pids=""
                    ;;
            esac
            running=$((running - 1))
        fi
    done < "$refs_file"

    rm -f "$refs_file"

    for pid in $pids; do
        wait "$pid" || true
    done

    log "Initial image sync completed"
}

watch_events() {
    log "Watching Docker image events and syncing to ${TARGET_PREFIX}"

    docker events \
        --format '{{json .}}' \
        --filter type=image \
        --filter event=tag \
        --filter event=pull \
        --filter event=import \
        --filter event=load \
    | while IFS= read -r event_json; do
        image_ref="$(printf '%s' "$event_json" | jq -r '.Actor.Attributes.name // empty')"
        image_id="$(printf '%s' "$event_json" | jq -r '.id // empty')"

        if [ -n "$image_ref" ]; then
            push_ref "$image_ref"
        fi

        if [ -n "$image_id" ]; then
            push_all_tags_for_image_id "$image_id"
        fi
    done
}

log "Starting with registry ${TARGET_PREFIX} (api: ${REGISTRY_API_ENDPOINT}, delete-before-push: ${DELETE_BEFORE_PUSH})"
docker version >/dev/null

if [ "$SYNC_EXISTING_ON_START" = "true" ]; then
    initial_sync
fi

watch_events

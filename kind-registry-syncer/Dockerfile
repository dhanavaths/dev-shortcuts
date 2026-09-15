FROM docker:27-cli

RUN apk add --no-cache jq curl coreutils

WORKDIR /app
COPY local-registry-image-syncer.sh /app/local-registry-image-syncer.sh

ENTRYPOINT ["sh", "/app/local-registry-image-syncer.sh"]

FROM ghcr.io/celestiaorg/celestia-app:v1.7.0 AS celestia-app

FROM ghcr.io/celestiaorg/celestia-node:v0.14.0

USER root

# hadolint ignore=DL3018
RUN apk --no-cache add \
        curl \
        jq \
        openssl \
    && mkdir /light 

COPY --from=celestia-app /bin/celestia-appd /bin/

COPY start.sh /opt/start.sh

EXPOSE 26657 26658 26659 9090

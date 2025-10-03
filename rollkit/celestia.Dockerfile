FROM ghcr.io/celestiaorg/celestia-app:v3.4.0-mocha AS celestia-app

FROM ghcr.io/celestiaorg/celestia-node:v0.27.5-mocha

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

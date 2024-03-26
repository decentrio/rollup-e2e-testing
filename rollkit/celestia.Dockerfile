FROM ghcr.io/celestiaorg/celestia-app:v1.7.0 AS celestia-app

FROM ghcr.io/celestiaorg/celestia-node:v0.13.1

USER root

# hadolint ignore=DL3018
RUN apk --no-cache add \
        curl \
        jq \
        openssl \
    && mkdir /bridge \
    && chown celestia:celestia /bridge

USER celestia

COPY --from=celestia-app /bin/celestia-appd /bin/

EXPOSE 26650 26657 26658 26659 9090
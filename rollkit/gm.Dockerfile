FROM docker.io/ignitehq/cli:v28.2.0 as ignite

FROM docker.io/golang:1.22.1-alpine3.18 as builder

# hadolint ignore=DL3018
RUN apk --no-cache add \
        libc6-compat

COPY --from=ignite /usr/bin/ignite /usr/bin/ignite

WORKDIR /apps

COPY --chown=1000:1000 gm .

RUN ignite chain build --release

FROM docker.io/alpine:3.18.3

# Read here why UID 10001: https://github.com/hexops/dockerfile/blob/main/README.md#do-not-use-a-uid-below-10000
ARG UID=10001
ARG USER_NAME=rollkit

ENV ROLLKIT_HOME=/home/${USER_NAME}

# hadolint ignore=DL3018
RUN apk --no-cache add \
        bash \
        libc6-compat \
    # Creates a user with $UID and $GID=$UID
    && adduser ${USER_NAME} \
        -D \
        -g ${USER_NAME} \
        -h ${ROLLKIT_HOME} \
        -s /sbin/nologin \
        -u ${UID}

COPY --from=builder /apps/release /release

WORKDIR /release

# Workaround for https://github.com/ignite/cli/issues/3480
# Fixed in https://github.com/ignite/cli/pull/3481
# hadolint ignore=DL4006
RUN file=$(find . -maxdepth 1 -regex '.*\.tar\.gz') \
    && sum=$(< release_checksum sed 's/ .*//') \
    && echo "$sum  $file" | sha256sum -c - \
    && tar -xvf "$file" -C /usr/bin/

WORKDIR ${ROLLKIT_HOME}

USER ${USER_NAME}

ENTRYPOINT [ "gmd" ]
# sandbox0 default sandbox template (runtime-only).
# This image intentionally does NOT define an ENTRYPOINT/CMD.
# The platform will inject and start the sandbox control process at runtime.

FROM ubuntu:latest

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    bash \
    zsh \
    python3 \
    python3-pip \
    python-is-python3 \
    nodejs \
    npm \
    ruby-full \
    lua \
    lua5.4 \
    luajit \
    php-cli \
    r-base \
    perl \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /root

# Intentionally no ENTRYPOINT/CMD here.

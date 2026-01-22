#!/bin/sh
set -eu

service="${SERVICE:-}"
if [ -z "$service" ]; then
  echo "SERVICE is required (e.g. edge-gateway, internal-gateway, manager, scheduler, storage-proxy, netd, k8s-plugin, infra-operator)" >&2
  exit 1
fi

case "$service" in
  edge-gateway|internal-gateway|manager|scheduler|storage-proxy|netd|k8s-plugin|infra-operator)
    exec "/usr/local/bin/$service" "$@"
    ;;
  *)
    echo "Unknown SERVICE: $service" >&2
    exit 1
    ;;
esac

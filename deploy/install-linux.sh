#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 1 ]] || { echo "usage: install-linux.sh /path/to/deployment.env" >&2; exit 2; }
[[ $(id -u) -eq 0 ]] || { echo "installation requires root" >&2; exit 1; }

SOURCE_DIRECTORY=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$SOURCE_DIRECTORY/lib.sh"
# shellcheck source=backup-lib.sh
source "$SOURCE_DIRECTORY/backup-lib.sh"
environment_source=$(configured_path "deployment environment source" "$1")
require_backup_control_file "deployment environment source" "$environment_source"
if paths_overlap "$environment_source" /etc/llm2api; then
  echo "deployment environment source must be outside /etc/llm2api" >&2
  exit 1
fi
load_llm2api_environment "$environment_source"
require_file_secrets
require_immutable_gateway_image
require_configuration_bindings /etc/llm2api

DEPLOY_DIRECTORY=/opt/llm2api/deploy
install -d -m 0750 /opt/llm2api "$DEPLOY_DIRECTORY" /etc/llm2api /etc/llm2api/secrets
install -m 0644 "$SOURCE_DIRECTORY/compose.production.yaml" "$DEPLOY_DIRECTORY/compose.production.yaml"
install -m 0644 "$SOURCE_DIRECTORY/Caddyfile" "$DEPLOY_DIRECTORY/Caddyfile"
install -m 0644 "$SOURCE_DIRECTORY/lib.sh" "$DEPLOY_DIRECTORY/lib.sh"
install -m 0755 "$SOURCE_DIRECTORY/upgrade-linux.sh" "$DEPLOY_DIRECTORY/upgrade-linux.sh"
install -m 0755 "$SOURCE_DIRECTORY/rotate-credentials-linux.sh" "$DEPLOY_DIRECTORY/rotate-credentials-linux.sh"
install -m 0640 "$environment_source" /etc/llm2api/deployment.env

gateway_secret_files=(
  "$LLM2API_DATABASE_URL_FILE"
  "$LLM2API_VALKEY_PASSWORD_FILE"
  "$LLM2API_MASTER_KEYS_FILE"
  "$LLM2API_SESSION_PEPPER_FILE"
  "$LLM2API_API_KEY_PEPPER_FILE"
  "$LLM2API_COORDINATION_KEY_HASH_SECRET_FILE"
)
chown 65532:65532 "${gateway_secret_files[@]}"
chmod 0400 "${gateway_secret_files[@]}"
chown 999:1000 "$LLM2API_VALKEY_ACL_FILE"
chmod 0400 "$LLM2API_VALKEY_ACL_FILE"
chown root:root "$LLM2API_POSTGRES_PASSWORD_FILE"
chmod 0400 "$LLM2API_POSTGRES_PASSWORD_FILE"
chown root:root /etc/llm2api /etc/llm2api/secrets /etc/llm2api/deployment.env
chmod 0750 /etc/llm2api /etc/llm2api/secrets
chmod 0640 /etc/llm2api/deployment.env
verify_runtime_configuration_tree /etc/llm2api

export DEPLOY_DIRECTORY
deployment_compose config --quiet
deployment_compose pull
deployment_compose up --detach --wait postgres valkey
deployment_compose --profile migration run --rm migrate

install -m 0644 "$SOURCE_DIRECTORY/llm2api-compose.service" /etc/systemd/system/llm2api-compose.service
systemctl daemon-reload
systemctl enable --now llm2api-compose.service
systemctl is-active --quiet llm2api-compose.service
deployment_compose ps
echo "LLM2API Linux production stack installed."

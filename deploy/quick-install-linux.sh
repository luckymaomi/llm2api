#!/usr/bin/env bash
set -euo pipefail

repository=${LLM2API_INSTALL_REPOSITORY:-luckymaomi/llm2api}
revision=${LLM2API_INSTALL_REVISION:-master}
source_directory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)

if [[ ! -f $source_directory/compose.production.yaml ]]; then
  temporary_directory=$(mktemp -d)
  trap 'rm -rf -- "$temporary_directory"' EXIT
  archive_url="https://github.com/${repository}/archive/refs/heads/${revision}.tar.gz"
  echo "正在下载 LLM2API ${revision}..."
  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error "$archive_url" | tar -xz -C "$temporary_directory"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$archive_url" | tar -xz -C "$temporary_directory"
  else
    echo "需要 curl 或 wget。Ubuntu: apt-get install -y curl" >&2
    exit 1
  fi
  downloaded_script=$(find "$temporary_directory" -path '*/deploy/quick-install-linux.sh' -type f -print -quit)
  [[ -n $downloaded_script ]] || { echo "下载包中没有安装脚本。" >&2; exit 1; }
  exec bash "$downloaded_script" "$@"
fi

[[ $(id -u) -eq 0 ]] || { echo "请使用 sudo 运行一键安装。" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "未安装 Docker。请先安装 Docker Engine。" >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "未安装 Docker Compose plugin。" >&2; exit 1; }
command -v openssl >/dev/null 2>&1 || { echo "未安装 openssl。Ubuntu: apt-get install -y openssl" >&2; exit 1; }

domain=${LLM2API_DOMAIN:-}
acme_email=${LLM2API_ACME_EMAIL:-}
gateway_tag=${LLM2API_GATEWAY_TAG:-ghcr.io/luckymaomi/llm2api:master}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) domain=${2:-}; shift 2 ;;
    --email) acme_email=${2:-}; shift 2 ;;
    --gateway-image) gateway_tag=${2:-}; shift 2 ;;
    *) echo "未知参数：$1" >&2; exit 2 ;;
  esac
done

if [[ -z $domain ]]; then
  read -r -p "请输入已经解析到本机公网 IP 的域名：" domain </dev/tty
fi
if [[ -z $acme_email ]]; then
  read -r -p "请输入用于 HTTPS 证书通知的邮箱：" acme_email </dev/tty
fi
[[ $domain =~ ^[A-Za-z0-9.-]+$ && $domain == *.* ]] || { echo "域名格式不正确。" >&2; exit 2; }
[[ $acme_email == *@*.* ]] || { echo "邮箱格式不正确。" >&2; exit 2; }

pin_image() {
  local tag=$1 digest
  echo "拉取镜像：$tag" >&2
  docker pull "$tag" >/dev/null
  digest=$(docker image inspect --format '{{index .RepoDigests 0}}' "$tag")
  [[ $digest =~ @sha256:[a-f0-9]{64}$ ]] || { echo "镜像 $tag 没有可固定的 digest。" >&2; exit 1; }
  printf '%s' "$digest"
}

gateway_image=$(pin_image "$gateway_tag")
postgres_image=$(pin_image "postgres:18.4-alpine")
valkey_image=$(pin_image "valkey/valkey:9.1.0-alpine")
caddy_image=$(pin_image "caddy:2.10.2-alpine")

configuration_root=/etc/llm2api
deploy_root=/opt/llm2api/deploy
if [[ -e $configuration_root/deployment.env ]]; then
  echo "检测到已有安装：$configuration_root/deployment.env" >&2
  echo "请使用 $deploy_root/manage-linux.sh start|stop|status|logs，不要覆盖安装。" >&2
  exit 1
fi

umask 077
install -d -o root -g root -m 0750 "$configuration_root" "$configuration_root/secrets" /opt/llm2api "$deploy_root"
install -m 0644 "$source_directory/compose.production.yaml" "$deploy_root/compose.production.yaml"
install -m 0644 "$source_directory/Caddyfile" "$deploy_root/Caddyfile"
install -m 0644 "$source_directory/lib.sh" "$deploy_root/lib.sh"
install -m 0644 "$source_directory/backup-lib.sh" "$deploy_root/backup-lib.sh"
install -m 0755 "$source_directory/manage-linux.sh" "$deploy_root/manage-linux.sh"

postgres_password=$(openssl rand -hex 24)
valkey_password=$(openssl rand -hex 24)
master_key=$(openssl rand -base64 32 | tr -d '\n')
session_pepper=$(openssl rand -hex 32)
api_key_pepper=$(openssl rand -hex 32)
coordination_secret=$(openssl rand -hex 32)

printf '%s' "$postgres_password" > "$configuration_root/secrets/postgres-password"
printf 'postgres://llm2api:%s@postgres:5432/llm2api?sslmode=disable' "$postgres_password" > "$configuration_root/secrets/database-url"
printf '%s' "$valkey_password" > "$configuration_root/secrets/valkey-password"
printf 'user default on >%s ~* &* +@all' "$valkey_password" > "$configuration_root/secrets/valkey-acl"
printf '1:%s' "$master_key" > "$configuration_root/secrets/master-keys"
printf '%s' "$session_pepper" > "$configuration_root/secrets/session-pepper"
printf '%s' "$api_key_pepper" > "$configuration_root/secrets/api-key-pepper"
printf '%s' "$coordination_secret" > "$configuration_root/secrets/coordination-secret"

cat > "$configuration_root/deployment.env" <<EOF
LLM2API_GATEWAY_IMAGE=$gateway_image
LLM2API_POSTGRES_IMAGE=$postgres_image
LLM2API_VALKEY_IMAGE=$valkey_image
LLM2API_CADDY_IMAGE=$caddy_image
LLM2API_ACTIVE_MASTER_KEY_VERSION=1
LLM2API_SITE_ADDRESS=$domain
LLM2API_ACME_EMAIL=$acme_email
LLM2API_POSTGRES_DB=llm2api
LLM2API_POSTGRES_USER=llm2api
LLM2API_POSTGRES_PASSWORD_FILE=$configuration_root/secrets/postgres-password
LLM2API_DATABASE_URL_FILE=$configuration_root/secrets/database-url
LLM2API_VALKEY_PASSWORD_FILE=$configuration_root/secrets/valkey-password
LLM2API_VALKEY_ACL_FILE=$configuration_root/secrets/valkey-acl
LLM2API_MASTER_KEYS_FILE=$configuration_root/secrets/master-keys
LLM2API_SESSION_PEPPER_FILE=$configuration_root/secrets/session-pepper
LLM2API_API_KEY_PEPPER_FILE=$configuration_root/secrets/api-key-pepper
LLM2API_COORDINATION_KEY_HASH_SECRET_FILE=$configuration_root/secrets/coordination-secret
EOF

chown 65532:65532 "$configuration_root/secrets/database-url" "$configuration_root/secrets/valkey-password" \
  "$configuration_root/secrets/master-keys" "$configuration_root/secrets/session-pepper" \
  "$configuration_root/secrets/api-key-pepper" "$configuration_root/secrets/coordination-secret"
chown 999:1000 "$configuration_root/secrets/valkey-acl"
chown root:root "$configuration_root/secrets/postgres-password" "$configuration_root/deployment.env"
chmod 0400 "$configuration_root/secrets/"*
chmod 0640 "$configuration_root/deployment.env"

set -a
# shellcheck disable=SC1090
source "$configuration_root/deployment.env"
set +a
export DEPLOY_DIRECTORY=$deploy_root
# shellcheck disable=SC1091
source "$deploy_root/lib.sh"
# shellcheck disable=SC1091
source "$deploy_root/backup-lib.sh"
require_file_secrets
require_immutable_gateway_image
require_configuration_bindings "$configuration_root"
deployment_compose config --quiet
deployment_compose up --detach --wait postgres valkey
deployment_compose --profile migration run --rm migrate
deployment_compose up --detach --wait

echo
echo "LLM2API 安装完成。"
echo "管理页面：https://$domain"
echo "首次打开页面后，输入管理员邮箱并保存只显示一次的初始密码。"
echo "查看状态：sudo $deploy_root/manage-linux.sh status"
echo "查看日志：sudo $deploy_root/manage-linux.sh logs 300"
echo "停止服务：sudo $deploy_root/manage-linux.sh stop"
echo "重新启动：sudo $deploy_root/manage-linux.sh start"

#!/usr/bin/env bash
set -euo pipefail

command=${1:-help}
environment_file=${LLM2API_ENVIRONMENT_FILE:-/etc/llm2api/deployment.env}
deploy_directory=${LLM2API_DEPLOY_DIRECTORY:-/opt/llm2api/deploy}

[[ -f $environment_file ]] || { echo "未找到 $environment_file，请先安装 LLM2API。" >&2; exit 1; }
[[ -f $deploy_directory/compose.production.yaml ]] || { echo "未找到生产 Compose 文件，请重新安装。" >&2; exit 1; }

set -a
# shellcheck disable=SC1090
source "$environment_file"
set +a
export DEPLOY_DIRECTORY=$deploy_directory
# shellcheck disable=SC1091
source "$deploy_directory/lib.sh"

health_url="https://${LLM2API_SITE_ADDRESS}/health/ready"

case "$command" in
  start)
    deployment_compose up --detach --wait
    echo "LLM2API 已启动： https://${LLM2API_SITE_ADDRESS}"
    ;;
  stop)
    deployment_compose stop
    echo "LLM2API 已停止，数据库文件和配置仍然保留。"
    ;;
  restart)
    deployment_compose up --detach --wait --force-recreate gateway-a gateway-b caddy
    echo "LLM2API 已重启。"
    ;;
  status)
    deployment_compose ps
    ;;
  logs)
    lines=${2:-200}
    [[ $lines =~ ^[1-9][0-9]{0,4}$ ]] || { echo "日志行数必须是 1 到 99999。" >&2; exit 2; }
    deployment_compose logs --tail "$lines"
    ;;
  follow)
    deployment_compose logs --follow --tail 100
    ;;
  health)
    curl --fail --silent --show-error "$health_url"
    printf '\n'
    ;;
  doctor)
    echo "== Docker"
    docker version --format 'Client {{.Client.Version}} / Server {{.Server.Version}}'
    docker compose version
    echo "== Compose 配置"
    deployment_compose config --quiet
    echo "配置可解析。"
    echo "== 容器状态"
    deployment_compose ps
    echo "== 公网健康检查"
    if curl --fail --silent --show-error "$health_url"; then
      printf '\n诊断通过。\n'
    else
      printf '\n健康检查失败。请运行：sudo %s logs 300\n' "$0" >&2
      exit 1
    fi
    ;;
  help|-h|--help)
    cat <<'EOF'
用法：sudo /opt/llm2api/deploy/manage-linux.sh 命令

  start       启动全部服务
  stop        停止服务但保留数据
  restart     重启两个 Gateway 和 Caddy
  status      查看每个容器是否健康
  logs [行数] 输出最近日志，例如 logs 300
  follow      持续查看实时日志，按 Ctrl+C 退出
  health      请求公网 readiness
  doctor      汇总 Docker、配置、容器和健康检查
EOF
    ;;
  *)
    echo "未知命令：$command。运行 $0 help 查看帮助。" >&2
    exit 2
    ;;
esac

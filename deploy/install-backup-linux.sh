#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 2 && $2 == --confirm-backup-schedule ]] || {
  echo "usage: $0 /etc/llm2api-backup/backup.env --confirm-backup-schedule" >&2
  exit 2
}
[[ $EUID -eq 0 ]] || { echo "backup schedule installation requires root" >&2; exit 1; }
SCRIPT_DIRECTORY=$(cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=backup-lib.sh
source "$SCRIPT_DIRECTORY/backup-lib.sh"
[[ $1 == /etc/llm2api-backup/backup.env && ! -L $1 ]] || {
  echo "production backup schedule requires /etc/llm2api-backup/backup.env" >&2
  exit 1
}

if [[ ! -e /var/lib/llm2api-backup && ! -L /var/lib/llm2api-backup ]]; then
  install -d -o 0 -g 0 -m 0700 /var/lib/llm2api-backup
fi
require_backup_directory "backup staging root" /var/lib/llm2api-backup
load_backup_environment "$1"
[[ $LLM2API_BACKUP_MODE == production ]] || { echo "scheduled backups require production mode" >&2; exit 1; }
[[ $LLM2API_CONFIGURATION_DIRECTORY == /etc/llm2api &&
   $LLM2API_DEPLOYMENT_ENVIRONMENT_FILE == /etc/llm2api/deployment.env &&
   $LLM2API_BACKUP_STAGING_ROOT == /var/lib/llm2api-backup &&
   $LLM2API_BACKUP_LAST_SUCCESS_MARKER_FILE == /var/lib/llm2api-backup/last-success &&
   $LLM2API_RESTIC_REPOSITORY_FILE == /etc/llm2api-backup/repository &&
   $LLM2API_RESTIC_PASSWORD_FILE == /etc/llm2api-backup/password &&
   $LLM2API_RESTIC_AWS_CREDENTIALS_FILE == /etc/llm2api-backup/aws-credentials ]] || {
  echo "scheduled backup paths do not match the fixed systemd sandbox" >&2
  exit 1
}
if [[ -n ${LLM2API_RESTIC_AWS_CONFIG_FILE:-} &&
      $LLM2API_RESTIC_AWS_CONFIG_FILE != /etc/llm2api-backup/aws-config ]]; then
  echo "scheduled AWS config path does not match the fixed systemd sandbox" >&2
  exit 1
fi

units=(
  llm2api-backup.timer
  llm2api-backup-freshness.timer
  llm2api-backup.service
  llm2api-backup-freshness.service
)

bundle_stage=''
link_stage=''
launcher_stage=''
unit_stages=()
maintenance_lock_held=false
cleanup() {
  local status=$? unit_stage
  trap - EXIT
  if [[ -n $link_stage && -L $link_stage ]]; then rm -f -- "$link_stage" || status=1; fi
  if [[ -n $launcher_stage && -f $launcher_stage && ! -L $launcher_stage ]]; then rm -f -- "$launcher_stage" || status=1; fi
  for unit_stage in "${unit_stages[@]}"; do
    if [[ -f $unit_stage && ! -L $unit_stage ]]; then rm -f -- "$unit_stage" || status=1; fi
  done
  if [[ -n $bundle_stage && -d $bundle_stage && ! -L $bundle_stage ]]; then rm -rf -- "$bundle_stage" || status=1; fi
  if [[ $maintenance_lock_held == true ]]; then release_llm2api_maintenance_lock || status=1; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

acquire_llm2api_maintenance_lock backup-installation
maintenance_lock_held=true
list_preflight_owner=backup-installation-$$-list
list_preflight_succeeded=true
if ! LLM2API_RESTIC_RUN_OWNER=$list_preflight_owner timeout --signal=TERM --kill-after=30s 5m \
    "$SCRIPT_DIRECTORY/list-backups-linux.sh" "$1" >/dev/null; then
  list_preflight_succeeded=false
fi
cleanup_restic_execution "$list_preflight_owner" || {
  echo "could not prove Restic list preflight container cleanup" >&2
  exit 1
}
if [[ $list_preflight_succeeded != true ]]; then
  echo "Restic repository must be initialized separately before schedule installation" >&2
  exit 1
fi
check_preflight_owner=backup-installation-$$-check
check_preflight_succeeded=true
if ! LLM2API_RESTIC_RUN_OWNER=$check_preflight_owner timeout --signal=TERM --kill-after=30s 20m \
    "$SCRIPT_DIRECTORY/check-restic-repository-linux.sh" "$1"; then
  check_preflight_succeeded=false
fi
cleanup_restic_execution "$check_preflight_owner" || {
  echo "could not prove Restic check preflight container cleanup" >&2
  exit 1
}
if [[ $check_preflight_succeeded != true ]]; then
  echo "Restic repository preflight check failed or timed out" >&2
  exit 1
fi

if [[ ! -e /opt/llm2api && ! -L /opt/llm2api ]]; then
  install -d -o 0 -g 0 -m 0750 /opt/llm2api
fi
[[ -d /opt/llm2api && ! -L /opt/llm2api && $(stat -c '%u:%g:%a' /opt/llm2api) == 0:0:750 ]] || {
  echo "/opt/llm2api must be a root-owned 0750 directory" >&2
  exit 1
}
require_root_owned_path_ancestors "backup bundle directory" /opt/llm2api/backup-active
require_root_owned_path_ancestors "systemd unit target" /etc/systemd/system/llm2api-backup.service
active_bundle_before_installation=''
if [[ -e /opt/llm2api/backup-active || -L /opt/llm2api/backup-active ]]; then
  [[ -L /opt/llm2api/backup-active ]] || { echo "active backup bundle must be a symbolic link" >&2; exit 1; }
  active_bundle_before_installation=$(realpath -e -- /opt/llm2api/backup-active)
  [[ $active_bundle_before_installation =~ ^/opt/llm2api/backup-bundle-[A-Za-z0-9]{8}$ &&
     -d $active_bundle_before_installation && ! -L $active_bundle_before_installation &&
     $(stat -c '%u:%g:%a' -- "$active_bundle_before_installation") == 0:0:750 ]] || {
    echo "installed backup bundle does not satisfy its runtime contract" >&2
    exit 1
  }
fi
remove_stale_private_directories /opt/llm2api .backup-bundle. '700|750'
bundle_stage=$(mktemp -d /opt/llm2api/.backup-bundle.XXXXXXXX)
bundle_suffix=${bundle_stage##*.}
bundle_directory=/opt/llm2api/backup-bundle-$bundle_suffix

for file in lib.sh backup-lib.sh compose.production.yaml Caddyfile; do
  install -m 0644 "$SCRIPT_DIRECTORY/$file" "$bundle_stage/$file"
done
for file in initialize-backup-linux.sh backup-linux.sh run-backup-with-retries-linux.sh \
  check-backup-freshness-linux.sh check-restic-repository-linux.sh list-backups-linux.sh restore-backup-linux.sh \
  restore-postgres-linux.sh install-restored-configuration-linux.sh install-backup-linux.sh; do
  install -m 0755 "$SCRIPT_DIRECTORY/$file" "$bundle_stage/$file"
done
for file in llm2api-backup.service llm2api-backup.timer \
  llm2api-backup-freshness.service llm2api-backup-freshness.timer; do
  install -m 0644 "$SCRIPT_DIRECTORY/$file" "$bundle_stage/$file"
done
chown -R 0:0 "$bundle_stage"
chmod 0750 "$bundle_stage"
bash -n "$bundle_stage"/*.sh
(
  load_llm2api_environment /etc/llm2api/deployment.env
  require_file_secrets
  require_immutable_gateway_image
  export DEPLOY_DIRECTORY=$bundle_stage
  deployment_compose config --quiet
)

mv -T -- "$bundle_stage" "$bundle_directory"
bundle_stage=''
link_stage=/opt/llm2api/.backup-active.$bundle_suffix
ln -s "$(basename -- "$bundle_directory")" "$link_stage"
mv -Tf -- "$link_stage" /opt/llm2api/backup-active
link_stage=''

launcher_stage=/opt/llm2api/.backup-bundle-launcher.$bundle_suffix
install -o 0 -g 0 -m 0755 "$SCRIPT_DIRECTORY/backup-bundle-launcher-linux.sh" "$launcher_stage"
mv -Tf -- "$launcher_stage" /opt/llm2api/backup-bundle-launcher-linux.sh
launcher_stage=''

for file in llm2api-backup.service llm2api-backup.timer \
  llm2api-backup-freshness.service llm2api-backup-freshness.timer; do
  [[ ! -d /etc/systemd/system/$file ]] || { echo "systemd unit target must not be a directory" >&2; exit 1; }
  unit_stage="/etc/systemd/system/.$file.$bundle_suffix"
  [[ ! -e $unit_stage && ! -L $unit_stage ]] || { echo "systemd unit staging path already exists" >&2; exit 1; }
  install -o 0 -g 0 -m 0644 "$bundle_directory/$file" "$unit_stage"
  unit_stages+=("$unit_stage")
done
for file in llm2api-backup.service llm2api-backup.timer \
  llm2api-backup-freshness.service llm2api-backup-freshness.timer; do
  unit_stage="/etc/systemd/system/.$file.$bundle_suffix"
  mv -Tf -- "$unit_stage" "/etc/systemd/system/$file"
done
unit_stages=()
systemctl daemon-reload
systemctl reset-failed "${units[@]}" >/dev/null 2>&1 || true
systemctl enable --now llm2api-backup.timer llm2api-backup-freshness.timer
release_llm2api_maintenance_lock
maintenance_lock_held=false
trap - EXIT INT TERM

initial_backup_succeeded=true
if ! systemctl restart llm2api-backup.service; then
  initial_backup_succeeded=false
elif [[ $(systemctl show --property=Result --value llm2api-backup.service) != success ]]; then
  initial_backup_succeeded=false
fi
systemctl is-active --quiet llm2api-backup.timer
systemctl is-active --quiet llm2api-backup-freshness.timer
[[ $initial_backup_succeeded == true ]] || {
  if [[ -n $active_bundle_before_installation ]]; then
    link_stage=/opt/llm2api/.backup-active.rollback.$bundle_suffix
    ln -s "$(basename -- "$active_bundle_before_installation")" "$link_stage"
    mv -Tf -- "$link_stage" /opt/llm2api/backup-active
    link_stage=''
    systemctl daemon-reload
    systemctl reset-failed llm2api-backup.service >/dev/null 2>&1 || true
    systemctl restart llm2api-backup.timer llm2api-backup-freshness.timer
    echo "initial scheduled backup failed; the active script bundle was rolled back and timers remain active" >&2
  else
    echo "initial scheduled backup failed; no earlier bundle exists and timers remain active for retry and freshness alerts" >&2
  fi
  exit 1
}
echo "LLM2API scheduled S3 backup and independent freshness monitoring installed."

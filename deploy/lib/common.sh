# shellcheck shell=bash
# Funções e caminhos usados por todos os scripts do servidor.
# Os scripts fazem: source /opt/ceasa-fruit/current/deploy/lib/common.sh

set -Eeuo pipefail

# --- Onde fica cada coisa no servidor --------------------------------------
APP_ROOT=/opt/ceasa-fruit                 # versões instaladas
RELEASES_DIR=$APP_ROOT/releases           # uma pasta por versão
CURRENT_LINK=$APP_ROOT/current            # aponta para a versão em uso
CONFIG_DIR=/etc/ceasa-fruit
APP_ENV=$CONFIG_DIR/ceasa.env             # configuração do sistema (API e front)
OPS_ENV=$CONFIG_DIR/ops.env               # backup, avisos, no-break (só root lê)
STATE_DIR=/var/lib/ceasa-fruit            # situação do backup, avisos enviados...
BACKUP_DIR=/var/backups/ceasa-fruit       # backups no disco do servidor
SPOOL_DIR=$BACKUP_DIR/spool               # último dump (antes de ir para o restic)
LOCAL_REPO=$BACKUP_DIR/restic             # repositório restic no disco do servidor
APP_USER=ceasa
DB_NAME=ceasa
DB_USER=ceasa

# --- Mensagens ---------------------------------------------------------------
if [[ -t 1 ]]; then
  _B=$'\e[1m'; _G=$'\e[32m'; _Y=$'\e[33m'; _R=$'\e[31m'; _N=$'\e[0m'
else
  _B=; _G=; _Y=; _R=; _N=
fi
info() { printf '%s==>%s %s\n' "$_B" "$_N" "$*"; }
ok()   { printf '%s ok%s %s\n' "$_G" "$_N" "$*"; }
warn() { printf '%saviso:%s %s\n' "$_Y" "$_N" "$*" >&2; }
die()  { printf '%serro:%s %s\n' "$_R" "$_N" "$*" >&2; exit 1; }

require_root() {
  [[ $EUID -eq 0 ]] || die "rode como administrador: sudo $0 $*"
}

# Carrega um arquivo KEY=valor do mesmo jeito que o systemd (EnvironmentFile):
# linha a linha, sem executar nada. Aspas em volta do valor são retiradas.
# (Não dá para usar "source": SMTP_FROM=CEASA Fruit <x@y> quebraria no "<".)
load_env() {
  local file=$1 line key value
  [[ -r $file ]] || return 0
  while IFS= read -r line || [[ -n $line ]]; do
    [[ $line =~ ^[[:space:]]*(#|$) ]] && continue
    [[ $line =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
    key=${BASH_REMATCH[1]} value=${BASH_REMATCH[2]}
    if [[ $value =~ ^\"(.*)\"$ || $value =~ ^\'(.*)\'$ ]]; then value=${BASH_REMATCH[1]}; fi
    export "$key=$value"
  done <"$file"
}

load_all_env() {
  load_env "$APP_ENV"
  load_env "$OPS_ENV"
}

# Atualiza (ou acrescenta) KEY=valor num arquivo .env, sem mexer no resto.
set_env_value() {
  local file=$1 key=$2 value=$3
  touch "$file"
  if grep -q "^${key}=" "$file"; then
    local tmp; tmp=$(mktemp)
    awk -v k="$key" -v v="$value" 'BEGIN{FS=OFS="="} $1==k {print k"="v; next} {print}' "$file" >"$tmp"
    cat "$tmp" >"$file"; rm -f "$tmp"
  else
    printf '%s=%s\n' "$key" "$value" >>"$file"
  fi
}

random_secret() { head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c "${1:-32}"; }

# psql como o superusuário do Postgres (socket local, sem senha).
psql_admin() { sudo -u postgres psql -v ON_ERROR_STOP=1 -qAt "$@"; }
# psql no banco do sistema.
psql_app() { sudo -u postgres psql -v ON_ERROR_STOP=1 -qAt -d "$DB_NAME" "$@"; }

# Registra no log do sistema (journalctl -t ceasa-fruit). Nunca derruba o script.
log_event() { logger -t ceasa-fruit "$*" 2>/dev/null || true; }

now_epoch() { date +%s; }
now_iso() { date --iso-8601=seconds; }

# Situação de cada rotina (backup, teste de restauração...), num arquivo
# simples que o ceasa-healthcheck e o ceasa-status leem:
#   $STATE_DIR/status/<nome>  ->  "ok|falhou <epoch> <mensagem>"
record_status() {
  local name=$1 result=$2; shift 2
  mkdir -p "$STATE_DIR/status"
  printf '%s %s %s\n' "$result" "$(now_epoch)" "$*" >"$STATE_DIR/status/$name"
  if [[ $result == ok ]]; then
    printf '%s\n' "$(now_epoch)" >"$STATE_DIR/status/$name.last_ok"
  fi
}

last_ok_epoch() {
  local f=$STATE_DIR/status/$1.last_ok
  [[ -r $f ]] && cat "$f" || echo 0
}

hours_since() { echo $(( ($(now_epoch) - $1) / 3600 )); }

# Repositórios de backup configurados: "nome caminho".
#   local  -> sempre (disco do servidor)
#   usb    -> HD externo, se BACKUP_USB_ENABLED=yes
#   remote -> nuvem, se RESTIC_REMOTE_REPOSITORY estiver preenchido
backup_targets() {
  echo "local $LOCAL_REPO"
  if [[ ${BACKUP_USB_ENABLED:-no} == yes ]]; then
    echo "usb ${BACKUP_USB_MOUNT:-/mnt/ceasa-backup}/restic"
  fi
  if [[ -n ${RESTIC_REMOTE_REPOSITORY:-} ]]; then
    echo "remote $RESTIC_REMOTE_REPOSITORY"
  fi
}

target_label() {
  case $1 in
    local) echo "disco do servidor" ;;
    usb) echo "HD externo" ;;
    remote) echo "nuvem" ;;
    *) echo "$1" ;;
  esac
}

# O HD externo está ligado e montado no lugar certo?
usb_ready() {
  local mnt=${BACKUP_USB_MOUNT:-/mnt/ceasa-backup}
  # Com automount a pasta sempre parece montada (autofs). Acessar a pasta
  # monta o HD, se ele estiver ligado; então conferimos o que ficou lá: o
  # ext4 do HD ("ext2/ext3" para o stat) ou só o autofs (HD desligado).
  # Sem o HD o acesso pode demorar: no máximo 15 segundos.
  [[ $(timeout 15 stat -f -c %T "$mnt/." 2>/dev/null) == ext2/ext3 ]]
}

# restic com a senha do ops.env. Uso: run_restic <repositório> <comando...>
# Com RESTIC_TIMEOUT=60 (segundos, ou 2h...), desiste depois desse tempo: sem
# isso, com a internet do box fora, o restic fica tentando a nuvem por muito tempo.
run_restic() {
  local repo=$1; shift
  local limit=()
  [[ -n ${RESTIC_TIMEOUT:-} ]] && limit=(timeout --kill-after=30s "$RESTIC_TIMEOUT")
  RESTIC_REPOSITORY=$repo RESTIC_PASSWORD=${RESTIC_PASSWORD:?senha do backup não configurada em $OPS_ENV} \
    "${limit[@]}" restic --no-cache "$@"
}

# O repositório responde? 0 = sim; 10 = ainda não existe; outro = inacessível.
restic_probe() {
  local repo=$1 rc=0
  RESTIC_TIMEOUT=${2:-60} run_restic "$repo" cat config >/dev/null 2>&1 || rc=$?
  return "$rc"
}

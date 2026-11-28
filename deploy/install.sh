#!/usr/bin/env bash
# Instala o CEASA Fruit num Ubuntu Server (26.04 LTS) limpo.
#
# Uso, com o pacote da versão descompactado:
#   sudo ./ceasa-fruit-<versão>/deploy/install.sh --admin-email voce@empresa.com.br
#
# Opções:
#   --admin-email E-MAIL   cria o primeiro administrador (só se não houver usuários)
#   --alert-email E-MAIL   para onde vão os avisos (backup, disco, no-break...)
#   --hosts "a b"          endereços do sistema na rede do box
#                          (padrão: <nome-do-servidor>.local e o IP da máquina)
#   --ups                  configura o no-break ligado no USB do servidor
#
# Pode rodar de novo quantas vezes quiser: nada é apagado, senhas e
# configurações existentes são mantidas. Para atualizar a versão, use
# ceasa-update (faz backup antes e volta sozinho se der problema).
RELEASE=$(cd "$(dirname "$0")/.." && pwd)
# shellcheck source=lib/common.sh
source "$RELEASE/deploy/lib/common.sh"
# shellcheck source=lib/setup.sh
source "$RELEASE/deploy/lib/setup.sh"
# shellcheck source=lib/notify.sh
source "$RELEASE/deploy/lib/notify.sh"

ADMIN_EMAIL='' ALERT='' HOSTS='' UPS=no
while [[ $# -gt 0 ]]; do
  case $1 in
    --admin-email) ADMIN_EMAIL=$2; shift 2 ;;
    --alert-email) ALERT=$2; shift 2 ;;
    --hosts) HOSTS=$2; shift 2 ;;
    --ups) UPS=yes; shift ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) die "opção desconhecida: $1 (veja --help)" ;;
  esac
done

require_root
[[ -x $RELEASE/bin/ceasa-api && -d $RELEASE/web ]] || die "rode a partir do pacote da versão (falta bin/ ou web/)"
APP_VERSION=$(cat "$RELEASE/VERSION")
os_name=$(sed -nE 's/^PRETTY_NAME="?([^"]*)"?$/\1/p' /etc/os-release)
grep -q '^ID=ubuntu' /etc/os-release || warn "feito para Ubuntu Server; este é $os_name"
info "Instalando CEASA Fruit $APP_VERSION em $os_name"

# --- 1. Pacotes do sistema --------------------------------------------------------
info "pacotes do sistema (Postgres, Caddy, Node, restic...)"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  postgresql caddy nodejs restic msmtp ca-certificates curl sudo \
  avahi-daemon smartmontools nut unattended-upgrades tzdata util-linux parted e2fsprogs >/dev/null
ok "pacotes instalados"

# Fuso horário do Rio (horários de backup e datas nos logs).
ln -sfn /usr/share/zoneinfo/America/Sao_Paulo /etc/localtime
echo America/Sao_Paulo >/etc/timezone
# Atualizações de segurança do Ubuntu automáticas.
printf 'APT::Periodic::Update-Package-Lists "1";\nAPT::Periodic::Unattended-Upgrade "1";\n' \
  >/etc/apt/apt.conf.d/20auto-upgrades

# --- 2. Usuário e pastas ---------------------------------------------------------------
if ! id "$APP_USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$STATE_DIR" --shell /usr/sbin/nologin "$APP_USER"
fi
install -d -m 0755 "$APP_ROOT" "$RELEASES_DIR"
install -d -m 0750 -o root -g "$APP_USER" "$CONFIG_DIR"
install -d -m 0755 "$STATE_DIR"
install -d -m 0700 "$BACKUP_DIR" "$SPOOL_DIR"

# --- 3. Esta versão em /opt/ceasa-fruit/releases --------------------------------------
target=$RELEASES_DIR/$APP_VERSION
if [[ -L $CURRENT_LINK && $(readlink -f "$CURRENT_LINK") != "$(readlink -f "$target")" ]]; then
  warn "a versão em uso é $(cat "$CURRENT_LINK/VERSION"); a instalação troca para $APP_VERSION (para atualizar com backup e volta automática, use ceasa-update)"
fi
if [[ $(readlink -f "$RELEASE") != "$(readlink -f "$target")" ]]; then
  rm -rf "$target.tmp" && cp -a "$RELEASE" "$target.tmp"
  rm -rf "$target" && mv "$target.tmp" "$target"
fi
chown -R root:root "$target"
ln -sfn "$target" "$CURRENT_LINK.tmp" && mv -T "$CURRENT_LINK.tmp" "$CURRENT_LINK"
ok "versão $APP_VERSION em $target"

# --- 4. Configuração (criada só na primeira vez) -----------------------------------------
if [[ -z $HOSTS ]]; then
  ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}' | head -1)
  HOSTS="$(hostname).local ${ip:-}"
fi
main_host=${HOSTS%% *}

if [[ ! -f $APP_ENV ]]; then
  info "criando $APP_ENV"
  db_password=$(random_secret 32)
  umask 027
  cat >"$APP_ENV" <<ENV
# Configuração do CEASA Fruit (API e front). Depois de mudar:
#   sudo systemctl restart ceasa-api ceasa-web
DATABASE_URL=postgres://$DB_USER:$db_password@localhost:5432/$DB_NAME?sslmode=disable
PORT=8080
APP_URL=https://$main_host
CORS_ORIGINS=https://$main_host
COOKIE_SECURE=true
WORKERS_ENABLED=true

# E-mail (convites, troca de senha e os avisos do servidor). Preencha com o
# SMTP do provedor de e-mail da empresa e rode a instalação de novo.
SMTP_HOST=
SMTP_PORT=587
SMTP_TLS=starttls
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=CEASA Fruit <nao-responda@exemplo.com.br>

# Rastreamento da frota: endereço e token do Traccar na nuvem
# (deploy/traccar/LEIAME.md). Vazio = frota só como cadastro, sem mapa ao vivo.
TRACCAR_URL=
TRACCAR_TOKEN=
FLEET_RETENTION_DAYS=180

# Chave que criptografa o certificado digital guardado no banco. Gerada na
# instalação e guardada também dentro dos backups (que são criptografados).
# NÃO TROQUE: o certificado já enviado deixa de abrir e precisa ser enviado de novo.
SECRETS_KEY=$(random_secret 48)
ENV
  umask 022
fi
chown root:"$APP_USER" "$APP_ENV" && chmod 0640 "$APP_ENV"
# Instalações anteriores ao certificado digital: cria a chave uma vez.
if ! grep -qE '^SECRETS_KEY=.{32,}' "$APP_ENV"; then
  info "criando a chave do certificado digital (SECRETS_KEY)"
  set_env_value "$APP_ENV" SECRETS_KEY "$(random_secret 48)"
fi

if [[ ! -f $OPS_ENV ]]; then
  info "criando $OPS_ENV"
  umask 077
  cat >"$OPS_ENV" <<ENV
# Backup, avisos e no-break. Só o administrador do servidor lê este arquivo.

# Endereços do sistema na rede do box (o primeiro é o principal).
SERVER_HOSTS="$HOSTS"

# Quem recebe os avisos (backup que falhou, disco cheio, falta de luz...).
ALERT_EMAIL=$ALERT

# Senha que criptografa TODOS os backups. Sem ela, nenhum backup pode ser
# restaurado: anote num lugar seguro FORA deste servidor.
RESTIC_PASSWORD=$(random_secret 40)

# HD externo (sudo ceasa-usb-setup prepara o disco e muda para yes).
BACKUP_USB_ENABLED=no
BACKUP_USB_MOUNT=/mnt/ceasa-backup

# Nuvem (opcional): qualquer destino do restic. Exemplos:
#   RESTIC_REMOTE_REPOSITORY=s3:https://s3.us-west-004.backblazeb2.com/meu-bucket/ceasa
#   AWS_ACCESS_KEY_ID=...
#   AWS_SECRET_ACCESS_KEY=...
RESTIC_REMOTE_REPOSITORY=

# Quantos backups guardar.
BACKUP_KEEP_LAST=8
BACKUP_KEEP_DAILY=7
BACKUP_KEEP_WEEKLY=4
BACKUP_KEEP_MONTHLY=12

# Avisar quando o último backup bem-sucedido for mais velho que (horas).
# A nuvem tem mais folga porque a internet do box cai.
ALERT_BACKUP_LOCAL_HOURS=12
ALERT_BACKUP_USB_HOURS=36
ALERT_BACKUP_REMOTE_HOURS=72
ALERT_DISK_PERCENT=85
# Rastreamento: avisar se as posições pararam de chegar por mais de (horas).
ALERT_FLEET_HOURS=6
# Certificado digital: avisar quantos dias antes de vencer (e todo dia depois).
ALERT_CERTIFICATE_DAYS=30

# No-break ligado no USB do servidor (install.sh --ups muda para yes).
UPS_ENABLED=no
ENV
  umask 022
fi
chmod 0600 "$OPS_ENV"
[[ -n $ALERT ]] && set_env_value "$OPS_ENV" ALERT_EMAIL "$ALERT"
[[ $UPS == yes ]] && set_env_value "$OPS_ENV" UPS_ENABLED yes
load_all_env

# --- 5. PostgreSQL -------------------------------------------------------------------------
info "PostgreSQL $(pg_major)"
render_postgres_conf
systemctl enable --quiet postgresql
systemctl restart postgresql
for _ in $(seq 1 30); do pg_isready -q && break; sleep 1; done
pg_isready -q || die "o PostgreSQL não subiu (journalctl -u postgresql)"
db_password=$(sed -E 's|^postgres://[^:]+:([^@]+)@.*|\1|' <<<"$DATABASE_URL")
setup_database "$db_password"
ok "banco $DB_NAME pronto (checksums de página: $(psql_admin -c 'SHOW data_checksums'))"

# --- 6. Comandos, serviços e e-mail -----------------------------------------------------------
install_commands
install_units "$CURRENT_LINK"
render_msmtp
render_nut

# --- 7. Sistema no ar ---------------------------------------------------------------------------
info "subindo o sistema"
systemctl restart ceasa-api
systemctl restart ceasa-web
render_caddy
systemctl enable --quiet caddy
systemctl restart caddy
enable_units
if ! wait_for_app 120; then
  journalctl -u ceasa-api -u ceasa-web -n 30 --no-pager >&2 || true
  die "o sistema não respondeu (logs acima)"
fi
publish_root_cert
ok "sistema no ar"

# --- 8. Primeiro administrador ---------------------------------------------------------------------
users=$(psql_app -c "SELECT count(*) FROM users")
admin_msg=
if [[ $users == 0 ]]; then
  if [[ -n $ADMIN_EMAIL ]]; then
    admin_msg=$(sudo -u "$APP_USER" env "DATABASE_URL=$DATABASE_URL" "$CURRENT_LINK/bin/ceasa-seed" -email "$ADMIN_EMAIL")
  else
    warn "nenhum usuário ainda: rode de novo com --admin-email voce@empresa.com.br"
  fi
fi

# --- 9. Primeiro backup ---------------------------------------------------------------------------
info "primeiro backup"
if /usr/local/sbin/ceasa-backup; then ok "backup feito"; else warn "o primeiro backup falhou (sudo ceasa-status)"; fi

# --- Resumo -------------------------------------------------------------------------------------
cat <<DONE

${_G}${_B}CEASA Fruit $APP_VERSION instalado.${_N}

  Endereço:      https://$main_host
  Outros:        $(for h in $SERVER_HOSTS; do printf 'https://%s ' "$h"; done)
  Certificado:   http://$main_host/certificado  (instale uma vez em cada aparelho)
  Situação:      sudo ceasa-status
DONE
if [[ -n $admin_msg ]]; then
  printf '\n%s\n' "$admin_msg" | sed 's/^/  /'
fi
cat <<DONE

${_Y}${_B}IMPORTANTE: anote a senha dos backups num lugar seguro, fora deste servidor.${_N}
  Sem ela nenhum backup pode ser restaurado. Para ver: sudo grep RESTIC_PASSWORD $OPS_ENV
DONE
[[ -z ${SMTP_HOST:-} ]] && warn "e-mail não configurado: preencha SMTP_* em $APP_ENV e rode a instalação de novo"
[[ -z ${ALERT_EMAIL:-} ]] && warn "ninguém recebe os avisos: rode de novo com --alert-email voce@empresa.com.br"
exit 0

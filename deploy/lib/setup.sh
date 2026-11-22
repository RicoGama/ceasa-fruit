# shellcheck shell=bash
# Passos de configuração usados pela instalação (install.sh) e pela
# atualização (ceasa-update). Todos podem rodar de novo sem estragar nada.
#
# $RELEASE é a pasta da versão (a que tem bin/, web/ e deploy/).

# --- Comandos ceasa-* em /usr/local/sbin ---------------------------------------
# São links para a versão em uso: atualizar o sistema atualiza os comandos.
install_commands() {
  local f name
  for f in "$CURRENT_LINK"/deploy/bin/ceasa-*; do
    name=$(basename "$f")
    ln -sfn "$f" "/usr/local/sbin/$name"
  done
}

# --- Serviços e rotinas (systemd) ----------------------------------------------
install_units() {
  local src=$1
  install -m 0644 "$src"/deploy/systemd/*.service "$src"/deploy/systemd/*.timer /etc/systemd/system/
  systemctl daemon-reload
}

enable_units() {
  systemctl enable --quiet ceasa-api.service ceasa-web.service
  systemctl enable --quiet --now ceasa-backup.timer ceasa-restore-test.timer ceasa-healthcheck.timer
}

# --- Caddy (HTTPS) --------------------------------------------------------------
# SERVER_HOSTS="ceasa-fruit.local 192.168.0.50" vira os endereços do site.
render_caddy() {
  local hosts=${SERVER_HOSTS:-localhost}
  local https_sites http_sites h
  https_sites=$(for h in $hosts localhost; do printf '%s ' "$h"; done | xargs -n1 | awk '!seen[$0]++' | paste -sd, | sed 's/,/, /g')
  http_sites=$(for h in $hosts; do printf 'http://%s ' "$h"; done | xargs -n1 | awk '!seen[$0]++' | paste -sd, | sed 's/,/, /g')
  [[ -n $http_sites ]] || http_sites="http://localhost"
  sed -e "s|__HTTPS_SITES__|$https_sites|" \
      -e "s|__HTTP_SITES__|$http_sites|" \
      -e "s|__APP_ROOT__|$APP_ROOT|" \
      "$CURRENT_LINK/deploy/caddy/Caddyfile.tmpl" >/etc/caddy/Caddyfile.new
  if caddy validate --config /etc/caddy/Caddyfile.new --adapter caddyfile >/dev/null 2>&1; then
    mv /etc/caddy/Caddyfile.new /etc/caddy/Caddyfile
  else
    caddy validate --config /etc/caddy/Caddyfile.new --adapter caddyfile || true
    die "Caddyfile inválido (veja acima); o anterior foi mantido"
  fi
}

# Copia o certificado raiz do Caddy para o endereço /certificado.
publish_root_cert() {
  local src=/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt
  local i
  for i in $(seq 1 30); do
    [[ -r $src ]] && break
    sleep 1
  done
  if [[ -r $src ]]; then
    install -d -m 0755 "$STATE_DIR/www"
    install -m 0644 "$src" "$STATE_DIR/www/ceasa-fruit-ca.crt"
  else
    warn "o Caddy ainda não gerou o certificado; rode a instalação de novo em 1 minuto"
  fi
}

# --- E-mail dos avisos (msmtp, com o SMTP do sistema) --------------------------
render_msmtp() {
  if [[ -z ${SMTP_HOST:-} || ${SMTP_HOST:-} == localhost ]]; then
    rm -f /etc/msmtprc
    return 0
  fi
  local from_addr tls starttls auth
  from_addr=$(sed -E 's/.*<([^>]+)>.*/\1/' <<<"${SMTP_FROM:-}")
  case ${SMTP_TLS:-starttls} in
    tls) tls=on; starttls=off ;;
    starttls) tls=on; starttls=on ;;
    *) tls=off; starttls=off ;;
  esac
  auth=$([[ -n ${SMTP_USERNAME:-} ]] && echo on || echo off)
  umask 077
  cat >/etc/msmtprc <<MSMTP
# Gerado pela instalação do CEASA Fruit a partir de /etc/ceasa-fruit/ceasa.env.
defaults
tls $tls
tls_starttls $starttls
tls_trust_file /etc/ssl/certs/ca-certificates.crt
syslog LOG_MAIL

account ceasa
host $SMTP_HOST
port ${SMTP_PORT:-587}
from $from_addr
auth $auth
$( [[ $auth == on ]] && printf 'user %s\npassword %s\n' "$SMTP_USERNAME" "$SMTP_PASSWORD" )

account default : ceasa
MSMTP
  umask 022
  chmod 0600 /etc/msmtprc
}

# --- PostgreSQL ------------------------------------------------------------------
pg_major() { ls /etc/postgresql | sort -n | tail -1; }

# Ajustes para um servidor dedicado com SSD, calculados pela memória.
render_postgres_conf() {
  local major mem_mb sb ecs mwm
  major=$(pg_major)
  mem_mb=$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)
  sb=$(( mem_mb / 4 )); (( sb > 8192 )) && sb=8192
  ecs=$(( mem_mb * 3 / 4 ))
  mwm=$(( mem_mb / 16 )); (( mwm > 1024 )) && mwm=1024
  cat >"/etc/postgresql/$major/main/conf.d/ceasa-fruit.conf" <<PGCONF
# Gerado pela instalação do CEASA Fruit (memória: ${mem_mb} MB). Não edite:
# para mudar algo, crie outro arquivo nesta pasta (ex.: zz-local.conf).
listen_addresses = 'localhost'
max_connections = 100
shared_buffers = ${sb}MB
effective_cache_size = ${ecs}MB
maintenance_work_mem = ${mwm}MB
work_mem = 16MB
random_page_cost = 1.1            # SSD/NVMe
effective_io_concurrency = 200
wal_compression = on
max_wal_size = 2GB
checkpoint_completion_target = 0.9
timezone = 'America/Sao_Paulo'
log_timezone = 'America/Sao_Paulo'
# Registra consultas lentas e esperas por trava (ajuda a achar gargalos).
log_min_duration_statement = 500ms
log_lock_waits = on
log_temp_files = 10MB
PGCONF
}

setup_database() {
  local password=$1
  psql_admin <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '$DB_USER') THEN
    CREATE ROLE $DB_USER LOGIN PASSWORD '$password';
  ELSE
    ALTER ROLE $DB_USER WITH LOGIN PASSWORD '$password';
  END IF;
END
\$\$;
SQL
  if [[ -z $(psql_admin -c "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'") ]]; then
    sudo -u postgres createdb -O "$DB_USER" -E UTF8 "$DB_NAME"
  fi
}

# Troca o banco inteiro pelo conteúdo de um dump (formato custom do pg_dump).
# Usado pela restauração e pela volta automática de uma atualização.
# Os serviços ceasa-api e ceasa-web precisam estar parados.
restore_database() {
  local dump=$1
  [[ -r $dump ]] || die "arquivo de backup não encontrado: $dump"
  info "recriando o banco $DB_NAME a partir de $(basename "$dump")"
  psql_admin -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB_NAME' AND pid <> pg_backend_pid()" >/dev/null
  sudo -u postgres dropdb --if-exists "$DB_NAME"
  sudo -u postgres createdb -O "$DB_USER" -E UTF8 "$DB_NAME"
  # --role: os objetos voltam a pertencer ao usuário do sistema.
  sudo -u postgres pg_restore --no-owner --no-privileges --role="$DB_USER" --exit-on-error -d "$DB_NAME" <"$dump"
}

# Versão das migrations aplicadas no banco (0 = banco vazio).
schema_version() {
  psql_app -c "SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied" 2>/dev/null || echo 0
}

# --- Saúde ------------------------------------------------------------------------
wait_for_url() {
  local url=$1 seconds=${2:-60} i
  for ((i = 0; i < seconds; i++)); do
    if curl -fsk -m 3 -o /dev/null "$url"; then return 0; fi
    sleep 1
  done
  return 1
}

wait_for_app() {
  wait_for_url http://127.0.0.1:8080/health "${1:-90}" &&
    wait_for_url http://127.0.0.1:3000/login 60 &&
    wait_for_url https://localhost/login 30
}

# --- No-break (NUT) ------------------------------------------------------------------
# Só com UPS_ENABLED=yes no ops.env (o no-break ligado no USB do servidor).
render_nut() {
  if [[ ${UPS_ENABLED:-no} != yes ]]; then
    return 0
  fi
  local pass
  pass=${UPS_MONITOR_PASSWORD:-}
  if [[ -z $pass ]]; then
    pass=$(random_secret 24)
    set_env_value "$OPS_ENV" UPS_MONITOR_PASSWORD "$pass"
  fi
  cat >/etc/nut/nut.conf <<'NUT'
# Gerado pelo CEASA Fruit: o no-break está ligado direto neste servidor.
MODE=standalone
NUT
  cat >/etc/nut/ups.conf <<'NUT'
# Gerado pelo CEASA Fruit. "usbhid-ups" atende a maioria dos no-breaks USB
# (APC, SMS, NHS...). Se o seu não for reconhecido: sudo nut-scanner -U
[nobreak]
  driver = usbhid-ups
  port = auto
  desc = "No-break do servidor"
NUT
  cat >/etc/nut/upsd.users <<NUT
[monitor]
  password = $pass
  upsmon primary
NUT
  cat >/etc/nut/upsmon.conf <<NUT
# Gerado pelo CEASA Fruit.
MONITOR nobreak@localhost 1 monitor $pass primary
MINSUPPLIES 1
# Bateria fraca: desliga o servidor com segurança (o Postgres fecha direito).
SHUTDOWNCMD "/sbin/shutdown -h +0"
NOTIFYCMD /usr/local/sbin/ceasa-ups-event
NOTIFYFLAG ONBATT SYSLOG+EXEC
NOTIFYFLAG ONLINE SYSLOG+EXEC
NOTIFYFLAG LOWBATT SYSLOG+EXEC
NOTIFYFLAG FSD SYSLOG+EXEC
NOTIFYFLAG COMMBAD SYSLOG+EXEC
NOTIFYFLAG COMMOK SYSLOG+EXEC
NOTIFYFLAG REPLBATT SYSLOG+EXEC
NOTIFYFLAG NOCOMM SYSLOG+EXEC
POLLFREQ 5
POLLFREQALERT 5
DEADTIME 15
FINALDELAY 5
NUT
  chown root:nut /etc/nut/*.conf /etc/nut/upsd.users
  chmod 0640 /etc/nut/*.conf /etc/nut/upsd.users
  systemctl enable --quiet nut-server.service nut-monitor.service 2>/dev/null || true
  systemctl restart nut-server.service nut-monitor.service 2>/dev/null ||
    warn "no-break não respondeu: confira o cabo USB e rode sudo nut-scanner -U"
}

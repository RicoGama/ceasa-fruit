# shellcheck shell=bash
# Envio de avisos por e-mail (msmtp, com o SMTP do ceasa.env).
# Uso: notify "assunto" "texto"   (sem ALERT_EMAIL: só registra no log)

notify() {
  local subject=$1 body=$2
  local host; host=$(hostname)
  log_event "AVISO: $subject"
  if [[ -z ${ALERT_EMAIL:-} ]]; then
    warn "sem ALERT_EMAIL em $OPS_ENV: aviso só no log ($subject)"
    return 0
  fi
  if [[ ! -r /etc/msmtprc ]]; then
    warn "e-mail do servidor não configurado (SMTP_* em $APP_ENV): aviso só no log ($subject)"
    return 0
  fi
  {
    printf 'From: %s\n' "${SMTP_FROM:-CEASA Fruit}"
    printf 'To: %s\n' "$ALERT_EMAIL"
    # Assunto com acento vai codificado (RFC 2047), senão chega embaralhado.
    printf 'Subject: =?UTF-8?B?%s?=\n' "$(printf '[CEASA Fruit - %s] %s' "$host" "$subject" | base64 -w0)"
    printf 'MIME-Version: 1.0\nContent-Type: text/plain; charset=UTF-8\nContent-Transfer-Encoding: 8bit\n\n'
    printf '%s\n\n' "$body"
    printf -- '-- \nServidor %s, %s\nPara ver a situação completa: sudo ceasa-status\n' "$host" "$(date '+%d/%m/%Y %H:%M')"
  } | msmtp --read-recipients 2>&1 || {
    warn "não consegui enviar o e-mail de aviso ($subject)"
    return 1
  }
}

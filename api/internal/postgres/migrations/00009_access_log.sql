-- Registro de acessos: quem entrou, quando, de onde, até quando ficou e como saiu.
--
-- Uma linha por sessão (kind = 'session') e uma por tentativa de login que
-- falhou (kind = 'failed_login'). A sessão "ativa" continua na tabela
-- sessions; aqui fica o histórico, que não é apagado quando a pessoa sai.
--
--   started_at    entrou (ou tentou entrar)
--   last_seen_at  última atividade: atualizada no máximo a cada minuto
--                 enquanto a pessoa usa o sistema ou está com a tela aberta
--   ended_at      saiu, com o motivo em end_reason:
--                   logout            clicou em "Sair"
--                   password_changed  a senha foi trocada (todas as sessões caem)
--                   deactivated       o usuário foi desativado
--                 Quem só fecha o navegador não tem ended_at: a tela mostra a
--                 última atividade (não dá para saber a hora exata em que saiu).
--   end_reason nas falhas: wrong_password, unknown_email, inactive_user

-- +goose Up
CREATE TABLE access_log (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('session', 'failed_login')),
    user_id      BIGINT REFERENCES users (id) ON DELETE SET NULL,
    email        TEXT NOT NULL,          -- o e-mail usado (mesmo se não existir)
    ip           TEXT,
    user_agent   TEXT,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ,
    end_reason   TEXT CHECK (end_reason IN (
                     'logout', 'password_changed', 'deactivated',
                     'wrong_password', 'unknown_email', 'inactive_user'))
);
CREATE INDEX access_log_started_idx ON access_log (started_at DESC);
CREATE INDEX access_log_user_idx ON access_log (user_id, started_at DESC);

-- Cada sessão aponta para a sua linha no histórico.
ALTER TABLE sessions ADD COLUMN access_log_id BIGINT REFERENCES access_log (id) ON DELETE SET NULL;

-- Tempo real: a tela de acessos atualiza quando alguém entra ou sai. Só em
-- INSERT e quando ended_at muda: a "última atividade" (a cada minuto, de cada
-- pessoa) não gera aviso.
CREATE TRIGGER access_log_notify AFTER INSERT OR UPDATE OF ended_at ON access_log
    FOR EACH ROW EXECUTE FUNCTION notify_change('access_log', 'id');

-- +goose Down
ALTER TABLE sessions DROP COLUMN access_log_id;
DROP TABLE access_log;

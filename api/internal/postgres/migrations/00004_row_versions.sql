-- Controle de concorrência otimista ("versão da linha").
--
-- Problema: duas pessoas abrem o mesmo cadastro; a primeira salva, a segunda
-- salva por cima sem saber, e a alteração da primeira some ("lost update").
--
-- Solução: cada linha editável tem um número de versão. A tela recebe a versão
-- ao abrir e a devolve ao salvar; o UPDATE só acontece se a versão ainda for a
-- mesma (WHERE version = @version) e então soma 1. Se outra pessoa salvou no
-- meio, nenhuma linha é alterada e a API responde 409.

-- +goose Up
ALTER TABLE parties ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE company ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users   ADD COLUMN version INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE users   DROP COLUMN version;
ALTER TABLE company DROP COLUMN version;
ALTER TABLE parties DROP COLUMN version;

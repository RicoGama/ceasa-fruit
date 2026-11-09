-- Prepara um PostgreSQL instalado direto na máquina (sem Docker) para o projeto.
-- Pode rodar quantas vezes quiser: só cria o que ainda não existe.
--
-- Uso (Linux): sudo -u postgres psql < scripts/db-setup-local.sql
--   ou, de dentro de api/: make db-setup-local

\set ON_ERROR_STOP on

-- Usuário da aplicação. SUPERUSER só em desenvolvimento: as migrations criam a
-- extensão unaccent e o papel ceasa_app. (Igual ao usuário que o Docker criaria.)
SELECT 'CREATE ROLE ceasa LOGIN SUPERUSER PASSWORD ''ceasa'''
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ceasa')
\gexec

-- \gexec executa o resultado do SELECT como comando. É o jeito de fazer um
-- "CREATE DATABASE se não existir", que o Postgres não tem pronto.
SELECT 'CREATE DATABASE ceasa OWNER ceasa'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'ceasa')
\gexec

-- Banco separado para os testes de integração (make test-integration).
SELECT 'CREATE DATABASE ceasa_test OWNER ceasa'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'ceasa_test')
\gexec

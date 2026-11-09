-- Roda só na PRIMEIRA vez que o volume do Postgres é criado.
-- Banco separado para os testes de integração (make test-integration),
-- para os testes não misturarem dados com o banco de desenvolvimento.
CREATE DATABASE ceasa_test;

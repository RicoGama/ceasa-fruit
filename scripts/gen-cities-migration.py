#!/usr/bin/env python3
"""Gera a migration de estados e municípios a partir da lista oficial do IBGE.

Uso:
    python3 scripts/gen-cities-migration.py municipios.json > api/internal/postgres/migrations/00003_create_states_and_cities.sql

O JSON vem de https://servicodados.ibge.gov.br/api/v1/localidades/municipios
(salve a resposta como arquivo). Para atualizar a lista no futuro (município
novo), gere uma NOVA migration com os INSERTs que faltarem; nunca edite uma
migration que já rodou.
"""
import json
import sys


def uf_of(m):
    # Municípios recentes podem vir sem "microrregiao"; nesse caso a UF está
    # na região imediata/intermediária (ex.: Boa Esperança do Norte - MT, 2025).
    if m.get("microrregiao"):
        return m["microrregiao"]["mesorregiao"]["UF"]
    return m["regiao-imediata"]["regiao-intermediaria"]["UF"]


def sql_str(s):
    return "'" + s.replace("'", "''") + "'"


def main():
    with open(sys.argv[1], encoding="utf-8") as f:
        cities = json.load(f)

    states = {}
    for m in cities:
        uf = uf_of(m)
        states[uf["sigla"]] = (uf["id"], uf["nome"])

    out = []
    out.append("-- Gerado por scripts/gen-cities-migration.py a partir da lista do IBGE.")
    out.append(f"-- {len(states)} UFs e {len(cities)} municípios. Não edite à mão.")
    out.append("")
    out.append("-- +goose Up")
    out.append("""-- Tabelas de referência, iguais para todas as organizações (sem RLS).
CREATE TABLE states (
    code    TEXT     PRIMARY KEY CHECK (code ~ '^[A-Z]{2}$'), -- sigla: RJ, SP...
    ibge_id SMALLINT NOT NULL UNIQUE,                         -- código IBGE da UF: 33 = RJ
    name    TEXT     NOT NULL
);

CREATE TABLE cities (
    -- Código IBGE de 7 dígitos (ex.: 3304557 = Rio de Janeiro). É o código que
    -- a nota fiscal eletrônica exige, por isso ele é a própria chave.
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    state_code TEXT    NOT NULL REFERENCES states (code)
);
CREATE INDEX cities_state_idx ON cities (state_code);

-- As transações de organização (papel ceasa_app) também leem estas tabelas.
GRANT SELECT ON states, cities TO ceasa_app;
""")
    out.append("INSERT INTO states (code, ibge_id, name) VALUES")
    rows = [f"    ({sql_str(code)}, {sid}, {sql_str(name)})" for code, (sid, name) in sorted(states.items())]
    out.append(",\n".join(rows) + ";")
    out.append("")

    cities = sorted(cities, key=lambda m: m["id"])
    chunk = 500
    for i in range(0, len(cities), chunk):
        out.append("INSERT INTO cities (id, name, state_code) VALUES")
        rows = [f"    ({m['id']}, {sql_str(m['nome'])}, {sql_str(uf_of(m)['sigla'])})" for m in cities[i:i + chunk]]
        out.append(",\n".join(rows) + ";")
        out.append("")

    out.append("-- +goose Down")
    out.append("DROP TABLE cities;")
    out.append("DROP TABLE states;")
    print("\n".join(out))


if __name__ == "__main__":
    main()

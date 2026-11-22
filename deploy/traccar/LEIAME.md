# Traccar na nuvem (rastreamento da frota)

Os rastreadores dos caminhões mandam a posição pela internet do celular para
um servidor que precisa estar no ar 24 horas, com endereço fixo. O servidor do
box não serve para isso: a internet de lá cai e ele não tem endereço fixo. Por
isso o [Traccar](https://www.traccar.org/) (gratuito, código aberto) fica numa
máquina pequena na nuvem, e o servidor do box busca as posições de lá.

```text
caminhão (FMC130 + chip M2M) ──► Traccar na nuvem ──► servidor do box ──► mapa
                                 (este compose)       (a cada 15 s)
```

Se a internet do box cair, nada se perde: o Traccar continua recebendo e, quando
a internet volta, o box busca tudo o que faltou. Sem sinal de celular na estrada,
o próprio rastreador guarda as posições e manda depois.

## 1. Máquina na nuvem

- 1 vCPU, 2 GB de memória, 20 GB de disco, Ubuntu Server 26.04 LTS. Qualquer
  provedor (DigitalOcean, Hetzner, AWS Lightsail, Magalu Cloud...).
- Docker instalado: `sudo apt install docker.io docker-compose-v2`.
- Um domínio apontando para o IP da máquina (registro A no DNS). Ex.:
  `rastreio.irmaosfaisao.com.br`.
- Portas liberadas no firewall do provedor: **80 e 443** (tela e API, HTTPS),
  **5027/TCP** (rastreadores Teltonika) e **5055/TCP** (app de teste, opcional).

## 2. Subir o Traccar

Copie esta pasta (`deploy/traccar`) para a máquina e rode:

```bash
cp exemplo.env .env
```

Preencha o `.env` (domínio e uma senha longa para o banco: `openssl rand -hex 24`) e:

```bash
docker compose up -d
```

Em um ou dois minutos, `https://<seu-domínio>` abre a tela do Traccar com
HTTPS de verdade (o Caddy pega o certificado no Let's Encrypt sozinho).

## 3. Administrador e token

1. Na tela do Traccar, **o primeiro cadastro vira o administrador**. Faça esse
   cadastro logo, antes de qualquer outra pessoa (depois dele, o cadastro fica
   fechado).
2. Em **Configurações → Preferências → Token**, gere um token sem data de
   validade (ou com validade longa) e copie.

## 4. Ligar o servidor do box

No servidor do box, em `/etc/ceasa-fruit/ceasa.env`:

```text
TRACCAR_URL=https://rastreio.irmaosfaisao.com.br
TRACCAR_TOKEN=<o token do passo 3>
```

```bash
sudo systemctl restart ceasa-api
```

Pronto: o mapa da frota deixa de mostrar "rastreamento não configurado". Os
veículos cadastrados no sistema **são criados sozinhos no Traccar** (nome =
placa, identificador = IMEI): não precisa cadastrar nada lá.

## 5. Configurar o rastreador (Teltonika FMC130 / FMC920)

Com o programa gratuito **Teltonika Configurator** (cabo USB no computador):

| Onde | O quê |
|---|---|
| GPRS → APN | o APN do chip M2M (vem com o chip) |
| GPRS → Server Settings → Domain | o domínio do Traccar (`rastreio.irmaosfaisao.com.br`) |
| GPRS → Server Settings → Port | `5027` |
| GPRS → Server Settings → Protocol | `TCP` |
| Data Acquisition → On Moving | enviar a cada `30` s (e a cada 200 m ou 15° de curva) |
| Data Acquisition → On Stop | enviar a cada `300` s (5 min) |

Grave no aparelho, coloque o chip e entregue para o auto-elétrico instalar
(positivo direto, terra e ignição). O IMEI está na etiqueta do aparelho: é o
que vai no cadastro do veículo, no campo "IMEI do rastreador".

## 6. Testar sem rastreador

O app **Traccar Client** (Android e iPhone, gratuito) transforma um celular em
rastreador:

1. No app, troque o "identificador do dispositivo" por um número de 15
   dígitos (ex.: `999000000000001`). Endereço do servidor:
   `http://<seu-domínio>:5055`, frequência 30 s.
2. No sistema, cadastre um veículo de teste com esse número no campo "IMEI do
   rastreador" e ligue o serviço no app.
3. Ande com o celular: o "veículo" aparece no mapa da frota.

## Manutenção

- Atualizar o Traccar: troque a versão em `docker-compose.yml` e rode
  `docker compose pull && docker compose up -d`.
- O histórico completo fica guardado no box (180 dias, `FLEET_RETENTION_DAYS`).
  O Traccar é só a "caixa de entrada" das posições.

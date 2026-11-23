# Servidor do CEASA Fruit — manual

Tudo o que é preciso para instalar, manter e recuperar o sistema no servidor
do box. Os comandos rodam no terminal do servidor (ou por SSH).

## Como o servidor funciona

```text
celulares e computadores do box
        │  https://<servidor>.local   (rede do box, não depende da internet)
        ▼
Caddy (HTTPS) ──► telas (Nuxt, Node) ──► API (Go) ──► PostgreSQL
                                                         │
                     ceasa-backup (4x por dia) ◄─────────┘
                        ├─► disco do servidor   (sempre)
                        ├─► HD externo USB      (se ligado)
                        └─► nuvem               (se configurada e com internet)
```

- Tudo sobe sozinho quando o servidor liga e reinicia sozinho se travar.
- A cada 5 minutos o servidor confere a própria saúde e manda e-mail se algo
  estiver errado (e outro quando se resolver).

## 1. Antes de instalar

1. **Ubuntu Server 26.04 LTS**, instalação mínima. Na tela de discos, com dois
   SSD/NVMe iguais, escolha **espelhamento (RAID 1)**: se um disco morrer, o
   outro segue com tudo.
2. **Nome do servidor**: por exemplo `ceasa-fruit`. O sistema fica em
   `https://ceasa-fruit.local`.
3. **IP fixo** no roteador (reserva de DHCP) ou no Ubuntu. Os aparelhos também
   podem usar o IP: `https://192.168.0.50`.
4. Na BIOS do servidor: **"ligar ao voltar a energia"** (*AC Power Recovery* /
   *Restore on AC Power Loss* = *On*).
5. No-break ligado no servidor pelo **cabo USB**.

## 2. Instalar

Copie o pacote da versão (`ceasa-fruit-<versão>.tar.gz`, gerado com
`deploy/build-release.sh`) para o servidor e rode:

```bash
tar xzf ceasa-fruit-*.tar.gz
```

```bash
sudo ./ceasa-fruit-*/deploy/install.sh --admin-email voce@empresa.com.br --alert-email ti@empresa.com.br --ups
```

- `--admin-email`: cria o primeiro administrador. A senha aparece na tela.
- `--alert-email`: quem recebe os avisos (backup, disco, falta de luz).
- `--ups`: configura o no-break (tire se ainda não houver).

No fim aparecem o endereço, o administrador e um aviso sobre a **senha dos
backups**. Anote essa senha num lugar seguro **fora do servidor** (cofre de
senhas, papel no cofre): sem ela nenhum backup pode ser restaurado.

```bash
sudo grep RESTIC_PASSWORD /etc/ceasa-fruit/ops.env
```

A instalação pode rodar de novo quantas vezes precisar: ela não apaga dados,
não troca senhas e mantém as configurações.

## 3. Certificado nos aparelhos (uma vez por aparelho)

O HTTPS usa um certificado próprio do servidor (a rede do box não tem domínio
na internet). Cada aparelho instala esse certificado uma vez:

1. No aparelho, abra `http://<servidor>.local/certificado` (ou pelo IP). O
   arquivo `ceasa-fruit-ca.crt` é baixado.
2. Instale:
   - **Android**: Configurações → Segurança → Criptografia e credenciais →
     Instalar certificado → Certificado CA.
   - **iPhone**: abra o arquivo baixado → Ajustes → Perfil baixado → Instalar.
     Depois: Ajustes → Geral → Sobre → Ajustes de confiança de certificados →
     ligar o "Caddy Local Authority".
   - **Windows**: dois cliques no arquivo → Instalar certificado → Máquina
     local → "Autoridades de Certificação Raiz Confiáveis".
3. Abra `https://<servidor>.local`: o cadeado aparece sem aviso.

## 4. E-mail

O sistema manda e-mails (convites, troca de senha) e o servidor manda os
avisos, pelo SMTP do provedor de e-mail da empresa. Preencha em
`/etc/ceasa-fruit/ceasa.env`:

```text
SMTP_HOST=smtp.provedor.com.br
SMTP_PORT=587
SMTP_TLS=starttls
SMTP_USERNAME=nao-responda@empresa.com.br
SMTP_PASSWORD=...
SMTP_FROM=CEASA Fruit <nao-responda@empresa.com.br>
```

Depois rode a instalação de novo (ela aplica o e-mail nos avisos e reinicia o
sistema).

## 5. Backup

| Onde | Quando | Guarda |
|---|---|---|
| Disco do servidor (`/var/backups/ceasa-fruit`) | 05:30, 11:30, 17:30 e 23:30 | últimos 8, 1 por dia por 7 dias, 1 por semana por 4 semanas, 1 por mês por 12 meses |
| HD externo USB | junto, se estiver ligado | a mesma regra |
| Nuvem | junto, se houver internet | a mesma regra |

- O backup **não para o sistema**: dá para rodar com gente trabalhando.
- Os backups são **criptografados** (restic) com a senha do item 2.
- **Todo domingo**, o servidor restaura o último backup num banco à parte e
  confere tabela por tabela que tudo voltou igual. Se não voltar, manda e-mail.
- Os horários ficam em `ceasa-backup.timer`. Para mudar:
  `sudo systemctl edit ceasa-backup.timer`.

### HD externo

Ligue o HD e prepare-o (isto **apaga** o HD):

```bash
sudo ceasa-usb-setup
```

```bash
sudo ceasa-usb-setup /dev/sdX
```

O primeiro comando lista os discos USB; o segundo prepara o escolhido. Dá
para preparar **dois HDs e revezar**: um fica no box, o outro sai (em casa).
O servidor usa o que estiver ligado. Se o HD ficar mais de 36 horas sem
backup, chega um e-mail.

### Nuvem

Qualquer destino que o restic aceite (Backblaze B2, Amazon S3, Wasabi...).
Preencha em `/etc/ceasa-fruit/ops.env`:

```text
RESTIC_REMOTE_REPOSITORY=s3:https://s3.us-west-004.backblazeb2.com/nome-do-bucket/ceasa
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
```

Sem internet, a cópia para a nuvem espera o próximo horário. O e-mail só
chega se a nuvem ficar mais de 72 horas sem backup.

### Restaurar

```bash
sudo ceasa-restore --list
```

```bash
sudo ceasa-restore
```

- `--list` mostra os backups com data e hora.
- Sem opções, restaura o **mais recente**. Para outro: `--snapshot <id>`. Do
  HD externo: `--from usb`. Da nuvem: `--from remote`.
- Antes de trocar o banco, o banco atual é guardado em
  `/var/backups/ceasa-fruit/before-restore-<data>.dump`. Para desfazer:
  `sudo ceasa-restore --file <esse arquivo>`.
- Pede para digitar `RESTAURAR`. O sistema fica fora do ar por alguns minutos.

**Servidor novo (o antigo queimou ou foi roubado):**

1. Instale como no item 2.
2. Troque a `RESTIC_PASSWORD` em `/etc/ceasa-fruit/ops.env` pela senha anotada
   e apague o backup local que a instalação criou com a senha nova:
   `sudo rm -rf /var/backups/ceasa-fruit/restic`.
3. Ligue o HD externo (ou configure a nuvem) e rode
   `sudo ceasa-restore --from usb` (ou `--from remote`).

## 6. Atualizar o sistema

```bash
sudo ceasa-update ceasa-fruit-<versão>.tar.gz
```

1. Faz um backup antes. Se o backup falhar, não atualiza.
2. Para o sistema. As telas mostram "o sistema está reiniciando" e voltam
   sozinhas.
3. Troca a versão e aplica as mudanças do banco.
4. Confere se tudo responde.

Se algo der errado, **volta sozinho** para a versão anterior. Se o banco já
tinha sido mudado, volta também o banco de antes. Chega um e-mail explicando.

Para voltar à versão anterior depois de uma atualização que deu certo:

```bash
sudo ceasa-update --rollback
```

## 7. Dia a dia

```bash
sudo ceasa-status
```

Mostra serviços, backups, teste de restauração, disco, espelhamento, saúde
dos discos e no-break. Os mesmos itens geram os e-mails de aviso.

| Aviso por e-mail | O que fazer |
|---|---|
| Serviço parado / telas não abrem | `sudo ceasa-status`; se continuar, `sudo systemctl restart ceasa-api ceasa-web caddy` |
| Backup FALHOU | `sudo ceasa-backup` e leia a mensagem; geralmente é disco cheio |
| Backup no HD externo atrasado | ligue o HD externo |
| Backup na nuvem atrasado | confira a internet do box |
| Disco com mais de 85% | apague arquivos que não são do sistema ou troque o disco por um maior |
| Espelhamento DEGRADADO / disco com defeito (SMART) | troque o disco com defeito **logo**: o servidor está sem cópia |
| SEM ENERGIA no box | nada: o no-break segura e, se a bateria acabar, o servidor desliga sozinho com segurança |
| E-mails do sistema NÃO foram enviados | convites ou trocas de senha não chegaram: confira o `SMTP_*` em `/etc/ceasa-fruit/ceasa.env` (senha trocada no provedor?) e reenvie em **Administração → Filas** |
| E-mails presos tentando de novo | o servidor de e-mail está recusando ou fora do ar: mesma conferência acima |
| Teste de restauração FALHOU | `sudo ceasa-restore-test` e leia a mensagem; os backups podem não estar servindo |

## 8. Frota (rastreamento)

As posições dos caminhões chegam por um Traccar na nuvem. Como montar, como
ligar no servidor do box (`TRACCAR_URL` e `TRACCAR_TOKEN` em
`/etc/ceasa-fruit/ceasa.env`) e como configurar os rastreadores:
[traccar/LEIAME.md](traccar/LEIAME.md).

Se as posições pararem de chegar por mais de 6 horas (internet do box fora
muito tempo, token trocado, Traccar fora do ar), chega um e-mail. Nada se
perde: quando a conexão volta, o box busca tudo o que faltou.

## 9. Comandos

| Comando | Para quê |
|---|---|
| `sudo ceasa-status` | situação de tudo |
| `sudo ceasa-backup` | backup agora |
| `sudo ceasa-restore-test` | testa o último backup agora |
| `sudo ceasa-restore` | restaura um backup (veja o item 5) |
| `sudo ceasa-update <pacote>` | atualiza o sistema (veja o item 6) |
| `sudo ceasa-usb-setup` | prepara um HD externo |
| `journalctl -u ceasa-api -f` | log da API, ao vivo |
| `journalctl -t ceasa-fruit` | registro das rotinas (backup, avisos, atualizações) |

Senha de um usuário esquecida, sem e-mail funcionando:

```bash
sudo -u ceasa env $(sudo grep ^DATABASE_URL /etc/ceasa-fruit/ceasa.env) /opt/ceasa-fruit/current/bin/ceasa-set-password -email pessoa@empresa.com.br
```

## 10. Onde fica cada coisa

| Caminho | O que é |
|---|---|
| `/opt/ceasa-fruit/current` | versão em uso (as anteriores ficam em `releases/`) |
| `/etc/ceasa-fruit/ceasa.env` | configuração do sistema (banco, e-mail, endereço) |
| `/etc/ceasa-fruit/ops.env` | backup, avisos e no-break (só o administrador lê) |
| `/var/backups/ceasa-fruit` | backups no disco do servidor |
| `/var/lib/ceasa-fruit` | situação das rotinas, avisos enviados, certificado |
| `/etc/postgresql/18/main/conf.d/ceasa-fruit.conf` | ajustes do PostgreSQL (calculados pela memória) |

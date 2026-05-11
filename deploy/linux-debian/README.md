# NetQuasar em Debian / Linux (Docker)

Este guia assume **Debian 11 ou 12** (ou derivado, ex. Ubuntu LTS) numa **VM ou bare metal**, por exemplo atrás de **Proxmox**. O anfitrião **Windows** continua a usar desenvolvimento com `go run` + `npm run dev`; aqui trata-se só do **deploy Linux** com a mesma raiz do repositório.

## Pré-requisitos

- CPU x86_64 (amd64), RAM recomendada **≥ 2 GiB** (4 GiB confortável para build e Postgres).
- Disco: **≥ 15 GiB** livres para imagens Docker e volume do Postgres.
- Utilizador com `sudo` ou root.

## 1. Instalar Docker Engine e o plugin Compose

Siga a documentação oficial (evita pacotes Debian desatualizados):

- **Debian:** [Install Docker Engine on Debian](https://docs.docker.com/engine/install/debian/)
- Confirme: `docker version` e `docker compose version`

Coloque o seu utilizador no grupo `docker` (opcional, evita `sudo` em cada comando):

```bash
sudo usermod -aG docker "$USER"
```

Saia da sessão SSH e volte a entrar para o grupo fazer efeito.

## 2. Obter o código

```bash
cd /opt   # ou $HOME/projetos
sudo git clone <URL_DO_REPOSITORIO> netquasar   # se usar /opt, ajuste dono: sudo chown -R $USER:$USER netquasar
cd netquasar
```

## 3. Ficheiro `.env` na raiz do projeto

O Compose lê **sempre** `.env` na mesma pasta que `docker-compose.yml`.

```bash
cp .env.example .env
nano .env   # ou vim
```

**Obrigatório ajustar em produção**

- `POSTGRES_PASSWORD` — senha forte.
- Opcional: `POSTGRES_USER`, `POSTGRES_DB`, `NETQUASAR_PUBLISH_PORT`.

**Evite erros comuns**

- Não commite `.env` (já está no `.gitignore` na raiz).
- O `docker-compose.yml` define `NETQUASAR_DATABASE_URL` vazio no serviço `netquasar` para **não** herdar por engano uma URL de Supabase/outro host copiada do teu `.env` de desenvolvimento Windows. O backend usa então `NETQUASAR_DB_HOST=postgres` e `POSTGRES_*`.
- Postgres **externo** ao Compose: cria um `docker-compose.override.yml` local (não versionado) ou um segundo ficheiro Compose em que removas essa linha e defines `NETQUASAR_DATABASE_URL` como precisares — cenário avançado.
- Caracteres especiais na palavra-passe: no `.env` sem aspas costuma ser mais simples; se usar aspas, mantém o par consistente.

Verificação rápida (na raiz do repo):

```bash
bash scripts/verify-compose-env.sh
```

## 4. Subir o sistema

Na **raiz** do repositório (onde está `docker-compose.yml`):

```bash
docker compose up -d --build
```

- Primeira vez: o build demora (npm + Go).
- Postgres fica saudável antes do `netquasar` arrancar (`depends_on` + healthcheck).

Logs:

```bash
docker compose logs -f netquasar
```

Parar (mantém volumes com dados):

```bash
docker compose down
```

## 5. Acesso e firewall

- Por omissão: `http://IP_DO_SERVIDOR:8080` (ou a porta em `NETQUASAR_PUBLISH_PORT`).
- Se usar `ufw` no Debian:

```bash
sudo ufw allow 8080/tcp comment 'NetQuasar'
sudo ufw reload
```

Ajuste a porta se alterou no `.env`.

## 6. Atualização (nova versão do código)

```bash
cd /caminho/netquasar
git pull
docker compose up -d --build
```

## 7. Backup

- Listar volumes: `docker volume ls | grep netquasar`
- Dump lógico (ajusta utilizador e base conforme o teu `.env`):

```bash
docker compose exec -T postgres pg_dump -U quasar netquasar > backup-$(date +%F).sql
```

Substitui `quasar` / `netquasar` se alteraste `POSTGRES_USER` / `POSTGRES_DB`.

## Estrutura relevante na raiz do repositório

| Ficheiro / pasta        | Função |
|-------------------------|--------|
| `docker-compose.yml`    | Stack: app + Postgres + Redis |
| `Dockerfile`            | Build multi-stage (Vite → Go → imagem Debian slim) |
| `.env.example`          | Modelo de variáveis |
| `.env`                  | **Local** — credenciais (não versionar) |
| `scripts/verify-compose-env.sh` | Verifica `.env` antes do `compose up` |

## Redis

O serviço `redis` sobe na mesma rede para uso futuro (cache / realtime). O processo `netquasar` atual pode ainda não ligar ao Redis; isso não impede o arranque.

## Proxmox

Recomenda-se **VM dedicada** (Debian 12) com Docker, em vez de instalar Docker diretamente no nó Proxmox, para isolamento e manutenção mais simples.

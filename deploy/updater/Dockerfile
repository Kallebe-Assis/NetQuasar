# updater — imagem mínima com git + Docker CLI/compose plugin + psql/jq/curl, usada só para
# executar deploy/updater/entrypoint.sh (ver docker-compose.yml, serviço `updater`).
FROM docker:27-cli
RUN apk add --no-cache git postgresql-client jq curl
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]

// Package buildinfo expõe a versão do binário em execução — preenchido via -ldflags -X no
// build (ver Dockerfile, stage "backend"). Um build sem esses argumentos (ex.: `go build`
// direto, sem passar por docker compose) fica com os valores por omissão "unknown", tratados
// como "sem informação" pelo endpoint /api/v1/system/version.
package buildinfo

var (
	GitCommit      = "unknown"
	GitCommitShort = "unknown"
	BuildTime      = "unknown"
)

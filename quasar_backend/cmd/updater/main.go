//go:build windows

// Command updater é o NetQuasarUpdater.exe — GUI de auto-atualização (duplo clique, sem
// terminal) que reimplementa em Go a mesma sequência segura do update.bat (raiz do projecto):
// verifica ferramentas, faz git fetch/pull --ff-only, reconstroi e sobe o stack Docker
// (docker compose up --build -d --remove-orphans) e confirma a saúde do Postgres e da API
// antes de reportar sucesso. Windows-only (usa github.com/lxn/walk, sem cgo).
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type appState struct {
	mw          *walk.MainWindow
	statusLbl   *walk.Label
	progressBar *walk.ProgressBar
	logBox      *walk.TextEdit
	checkBtn    *walk.PushButton
	updateBtn   *walk.PushButton
	closeBtn    *walk.PushButton

	rootDir     string
	composeBin  []string // ["docker","compose"] ou ["docker-compose"]
	composeArgs []string // -f docker-compose.yml [-f deploy/linux-debian/docker-compose.caddy.yml]
	env         map[string]string
	appPort     string
	busy        bool
}

func main() {
	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = filepath.Abs(os.Args[0])
	}
	app := &appState{rootDir: filepath.Dir(exePath), appPort: "8080"}

	if err := (MainWindow{
		AssignTo: &app.mw,
		Title:    "NetQuasar — Atualização",
		MinSize:  Size{Width: 680, Height: 520},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
		Children: []Widget{
			Label{Text: "Pasta do projecto:"},
			Label{Text: app.rootDir, TextColor: walk.RGB(120, 120, 120)},
			Label{AssignTo: &app.statusLbl, Text: "Pronto. Clique em «Verificar atualização» ou «Atualizar agora»."},
			ProgressBar{AssignTo: &app.progressBar, MinValue: 0, MaxValue: 100, Value: 0},
			TextEdit{
				AssignTo: &app.logBox,
				ReadOnly: true,
				VScroll:  true,
				MinSize:  Size{Height: 320},
				Font:     Font{Family: "Consolas", PointSize: 9},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						AssignTo: &app.checkBtn,
						Text:     "Verificar atualização",
						OnClicked: func() {
							go app.runGuarded(app.doCheck)
						},
					},
					PushButton{
						AssignTo: &app.updateBtn,
						Text:     "Atualizar agora",
						OnClicked: func() {
							if walk.MsgBox(app.mw, "Confirmar atualização",
								"Isto vai fazer git pull (se houver commits novos) e reconstruir/reiniciar o Docker.\n\nO sistema fica indisponível por alguns instantes durante o restart. Continuar?",
								walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
								return
							}
							go app.runGuarded(app.doUpdate)
						},
					},
					HSpacer{},
					PushButton{
						AssignTo:  &app.closeBtn,
						Text:      "Fechar",
						OnClicked: func() { app.mw.Close() },
					},
				},
			},
		},
	}).Create(); err != nil {
		panic(err)
	}

	app.mw.Run()
}

// runGuarded impede cliques concorrentes (só uma operação de cada vez) e devolve os botões
// ao estado normal no final, mesmo se a função entrar em pânico.
func (a *appState) runGuarded(fn func()) {
	if a.busy {
		return
	}
	a.busy = true
	a.mw.Synchronize(func() {
		a.checkBtn.SetEnabled(false)
		a.updateBtn.SetEnabled(false)
	})
	defer func() {
		a.busy = false
		a.mw.Synchronize(func() {
			a.checkBtn.SetEnabled(true)
			a.updateBtn.SetEnabled(true)
		})
	}()
	fn()
}

func (a *appState) log(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	a.mw.Synchronize(func() {
		a.logBox.AppendText(line + "\r\n")
	})
}

func (a *appState) setStatus(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	a.mw.Synchronize(func() {
		a.statusLbl.SetText(text)
	})
}

func (a *appState) setProgress(pct int) {
	a.mw.Synchronize(func() {
		if a.progressBar.MarqueeMode() {
			a.progressBar.SetMarqueeMode(false)
		}
		a.progressBar.SetValue(pct)
	})
}

func (a *appState) setMarquee(on bool) {
	a.mw.Synchronize(func() {
		a.progressBar.SetMarqueeMode(on)
	})
}

func (a *appState) fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.log("ERRO: %s", msg)
	a.setStatus("Falhou: %s", msg)
	a.setProgress(0)
	a.mw.Synchronize(func() {
		walk.MsgBox(a.mw, "Falhou", msg, walk.MsgBoxIconError)
	})
}

// runStreamed executa um comando na pasta do projecto, transmitindo cada linha de
// stdout/stderr para o log em tempo real. extraEnv (opcional, "CHAVE=valor") é acrescentado
// ao ambiente herdado — usado para DOCKER_BUILDKIT/CACHEBUST no passo de build.
func (a *appState) runStreamed(ctx context.Context, name string, args []string, extraEnv ...string) error {
	a.log("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = a.rootDir
	cmd.Env = append(os.Environ(), extraEnv...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{}, 2)
	pipe := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			a.log("%s", sc.Text())
		}
		done <- struct{}{}
	}
	go pipe(stdout)
	go pipe(stderr)
	<-done
	<-done
	return cmd.Wait()
}

// runCapture executa um comando e devolve stdout (trim), sem transmitir para o log — para
// leituras rápidas (git rev-list --count, etc.) cujo output bruto não interessa ao utilizador.
func (a *appState) runCapture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = a.rootDir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func commandExists(ctx context.Context, name string) bool {
	cmd := exec.CommandContext(ctx, name, "--version")
	return cmd.Run() == nil
}

func readEnvFile(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.Trim(strings.TrimSpace(line[i+1:]), `"`)
		out[key] = val
	}
	return out
}

// ---- Fase 1: verificações (ferramentas, ficheiros, .env) — mirror de update.bat :step blocos. ----

func (a *appState) prepare(ctx context.Context) bool {
	a.setStatus("A verificar ferramentas (git, docker, compose)…")
	a.log("--- Ferramentas ---")
	if !commandExists(ctx, "git") {
		a.fail(`comando "git" não encontrado no PATH.`)
		return false
	}
	if !commandExists(ctx, "docker") {
		a.fail(`comando "docker" não encontrado no PATH.`)
		return false
	}
	if _, err := a.runCapture(ctx, "docker", "compose", "version"); err == nil {
		a.composeBin = []string{"docker", "compose"}
	} else if _, err := a.runCapture(ctx, "docker-compose", "version"); err == nil {
		a.composeBin = []string{"docker-compose"}
	} else {
		a.fail(`nem "docker compose" nem "docker-compose" estão disponíveis.`)
		return false
	}
	a.log("Compose: %s", strings.Join(a.composeBin, " "))

	a.setStatus("A verificar se o Docker está a correr…")
	if _, err := a.runCapture(ctx, "docker", "info"); err != nil {
		a.fail("o Docker não está a correr. Inicie o Docker Desktop e tente de novo.")
		return false
	}
	a.log("Docker OK.")

	a.setStatus("A verificar ficheiros do projecto…")
	for _, f := range []string{"docker-compose.yml", "Dockerfile", ".env"} {
		if _, err := os.Stat(filepath.Join(a.rootDir, f)); err != nil {
			a.fail("%s não encontrado em %s.", f, a.rootDir)
			return false
		}
	}
	a.env = readEnvFile(filepath.Join(a.rootDir, ".env"))
	for _, k := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if strings.TrimSpace(a.env[k]) == "" {
			a.fail("variável %s ausente ou vazia no .env.", k)
			return false
		}
	}
	if p := strings.TrimSpace(a.env["NETQUASAR_PUBLISH_PORT"]); p != "" {
		a.appPort = p
	}

	a.composeArgs = []string{"-f", "docker-compose.yml"}
	caddyFile := filepath.Join("deploy", "linux-debian", "docker-compose.caddy.yml")
	if strings.TrimSpace(a.env["NETQUASAR_DOMAIN"]) != "" {
		if _, err := os.Stat(filepath.Join(a.rootDir, caddyFile)); err == nil {
			a.composeArgs = append(a.composeArgs, "-f", filepath.ToSlash(caddyFile))
			a.log("NETQUASAR_DOMAIN definido — a incluir o Caddy (HTTPS) no stack.")
		}
	} else {
		a.log("NETQUASAR_DOMAIN não definido — a subir só a app (HTTP na porta %s).", a.appPort)
	}
	a.log(".env e ficheiros OK. Porta da app: %s", a.appPort)
	return true
}

func (a *appState) composeExec(ctx context.Context, args ...string) error {
	full := append(append([]string{}, a.composeBin[1:]...), append(a.composeArgs, args...)...)
	return a.runStreamed(ctx, a.composeBin[0], full)
}

// ---- Fase 2: Git — mirror do bloco "Repositorio Git" do update.bat. ----

// gitStatus devolve (behind, ahead, remoteRef, error).
func (a *appState) gitStatus(ctx context.Context) (int, int, string, error) {
	if _, err := a.runCapture(ctx, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		return 0, 0, "", fmt.Errorf("%s não é um repositório git", a.rootDir)
	}
	if _, err := a.runCapture(ctx, "git", "remote", "get-url", "origin"); err != nil {
		return 0, 0, "", fmt.Errorf(`falta o remoto "origin"`)
	}
	a.log("git fetch --prune…")
	if err := a.runStreamed(ctx, "git", []string{"fetch", "--prune"}); err != nil {
		return 0, 0, "", fmt.Errorf("git fetch falhou (rede, credenciais ou remoto)")
	}

	remoteRef := ""
	if _, err := a.runCapture(ctx, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/main"); err == nil {
		remoteRef = "origin/main"
	} else if _, err := a.runCapture(ctx, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/master"); err == nil {
		remoteRef = "origin/master"
	} else {
		return 0, 0, "", fmt.Errorf("o remoto não tem ramo main nem master")
	}

	branch, _ := a.runCapture(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if _, err := a.runCapture(ctx, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err != nil {
		a.log("Sem upstream — a apontar %s para %s…", branch, remoteRef)
		if _, err := a.runCapture(ctx, "git", "branch", fmt.Sprintf("--set-upstream-to=%s", remoteRef), branch); err != nil {
			return 0, 0, "", fmt.Errorf("não foi possível definir upstream para %s", remoteRef)
		}
	}

	behindStr, err := a.runCapture(ctx, "git", "rev-list", "--count", "HEAD..@{u}")
	if err != nil {
		return 0, 0, "", fmt.Errorf("falha ao calcular commits em atraso: %w", err)
	}
	aheadStr, err := a.runCapture(ctx, "git", "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		return 0, 0, "", fmt.Errorf("falha ao calcular commits à frente: %w", err)
	}
	behind, _ := strconv.Atoi(behindStr)
	ahead, _ := strconv.Atoi(aheadStr)
	return behind, ahead, remoteRef, nil
}

// ---- Botão "Verificar atualização": só reporta o estado, não pulla nem toca no Docker. ----

func (a *appState) doCheck() {
	ctx := context.Background()
	a.setProgress(0)
	if !a.prepare(ctx) {
		return
	}
	a.setStatus("A verificar o Git…")
	behind, ahead, remoteRef, err := a.gitStatus(ctx)
	if err != nil {
		a.fail("%s", err.Error())
		return
	}
	a.setProgress(100)
	switch {
	case ahead > 0 && behind > 0:
		a.setStatus("Histórico divergiu de %s — resolva manualmente antes de actualizar.", remoteRef)
	case ahead > 0:
		a.setStatus("Há %d commit(s) local(is) por enviar (push) — «Atualizar agora» não vai puxar nada.", ahead)
	case behind == 0:
		a.setStatus("Já está actualizado com %s.", remoteRef)
	default:
		a.setStatus("Há %d commit(s) novo(s) em %s. Clique «Atualizar agora» para aplicar.", behind, remoteRef)
	}
	a.log("--- Verificação concluída ---")
}

// ---- Botão "Atualizar agora": fluxo completo, mirror do update.bat. ----

func (a *appState) doUpdate() {
	ctx := context.Background()
	a.setProgress(0)
	a.setStatus("A verificar ferramentas e ficheiros…")
	if !a.prepare(ctx) {
		return
	}
	a.setProgress(10)

	a.setStatus("A verificar o repositório Git…")
	behind, ahead, remoteRef, err := a.gitStatus(ctx)
	if err != nil {
		a.fail("%s", err.Error())
		return
	}
	if ahead > 0 && behind > 0 {
		a.fail("histórico divergiu de %s. Resolva o Git manualmente antes de actualizar.", remoteRef)
		return
	}
	if ahead > 0 {
		a.fail("há %d commit(s) local(is) que ainda não foram enviados (push/rebase necessário).", ahead)
		return
	}
	if dirty, _ := a.runCapture(ctx, "git", "status", "--porcelain"); strings.TrimSpace(dirty) != "" {
		a.log("AVISO: há alterações locais por commitar — o pull pode falhar se o remoto tocar nos mesmos ficheiros.")
	}
	if behind == 0 {
		a.log("Já está actualizado com %s.", remoteRef)
	} else {
		a.setStatus("A puxar %d commit(s) novo(s) de %s…", behind, remoteRef)
		if err := a.runStreamed(ctx, "git", []string{"pull", "--ff-only"}); err != nil {
			a.fail("git pull --ff-only falhou.")
			return
		}
	}
	commit, _ := a.runCapture(ctx, "git", "log", "-1", "--oneline")
	a.log("Commit actual: %s", commit)
	a.setProgress(30)

	a.setStatus("A reconstruir e subir o stack Docker (pode demorar vários minutos)…")
	cacheBust := strconv.FormatInt(time.Now().Unix(), 10)
	a.log("CACHEBUST=%s (força rebuild da UI/backend)", cacheBust)

	a.log("A puxar imagens de postgres/redis…")
	if err := a.composeExec(ctx, "pull", "postgres", "redis"); err != nil {
		a.log("AVISO: pull das imagens base falhou — a continuar com o que já existir em cache.")
	}

	a.setMarquee(true)
	buildArgs := append(append([]string{}, a.composeBin[1:]...), append(a.composeArgs, "up", "--build", "-d", "--remove-orphans")...)
	buildErr := a.runStreamed(ctx, a.composeBin[0], buildArgs, "DOCKER_BUILDKIT=1", "COMPOSE_DOCKER_CLI_BUILD=1", "CACHEBUST="+cacheBust)
	a.setMarquee(false)
	if buildErr != nil {
		a.log("--- últimos logs netquasar ---")
		_ = a.composeExec(ctx, "logs", "--tail=80", "netquasar")
		a.fail("docker compose up falhou.")
		return
	}
	a.setProgress(90)

	a.setStatus("A aguardar o Postgres ficar pronto…")
	pgOK := false
	for i := 0; i < 30; i++ {
		if _, err := a.runCapture(ctx, a.composeBin[0], append(append([]string{}, a.composeBin[1:]...), append(a.composeArgs, "exec", "-T", "postgres", "pg_isready", "-U", a.env["POSTGRES_USER"], "-d", a.env["POSTGRES_DB"])...)...); err == nil {
			pgOK = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !pgOK {
		a.fail("o Postgres não ficou pronto a tempo.")
		return
	}
	a.log("Postgres OK.")

	a.setStatus("A confirmar a saúde da API (/health)…")
	healthURL := fmt.Sprintf("http://127.0.0.1:%s/health", a.appPort)
	appOK := false
	client := &http.Client{Timeout: 3 * time.Second}
	for i := 0; i < 45; i++ {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				appOK = true
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if !appOK {
		a.log("--- últimos logs netquasar ---")
		_ = a.composeExec(ctx, "logs", "--tail=80", "netquasar")
		a.fail("a API não respondeu em %s a tempo.", healthURL)
		return
	}
	a.setProgress(100)
	a.setStatus("Concluído — UI/API em http://127.0.0.1:%s", a.appPort)
	a.log("--- Concluído com sucesso ---")
	a.mw.Synchronize(func() {
		walk.MsgBox(a.mw, "Actualização concluída",
			fmt.Sprintf("NetQuasar actualizado e disponível em http://127.0.0.1:%s", a.appPort),
			walk.MsgBoxIconInformation)
	})
}

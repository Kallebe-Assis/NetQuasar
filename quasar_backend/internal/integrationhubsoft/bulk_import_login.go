// Configuração do login PPPoE do serviço (configurar_autenticacao), observação de rastreio e reparo
// de senha provisória.

package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// LoginConfigResult — resultado de ApplyLoginConfig, deliberadamente um tipo PRÓPRIO (não reaproveita
// ImportApplyResult) para nunca ser confundido com o resultado da criação do cliente/serviço: o
// chamador grava os dois lado a lado no mesmo registro de histórico, nunca um substituindo o outro.
type LoginConfigResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// withNoAuthHint acrescenta a causa provável quando a HubSoft diz que o serviço não tem dados de
// autenticação: sem a automação de login automático ativa, nenhum serviço criado pela API nasce com
// registro de autenticação para a configurar_autenticacao alterar.
func withNoAuthHint(msg string) string {
	if strings.Contains(strings.ToLower(msg), "dados de autentica") {
		return msg + " — ative na HubSoft a automação que gera logins automaticamente (máscara de login); sem ela, serviços criados pela API nascem sem autenticação"
	}
	return msg
}

// ApplyLoginConfig — POST /api/v1/integracao/cliente/configurar_autenticacao, chamado pelo handler
// HTTP depois que ApplyClientImportRow/ApplyServiceImportRow cria o cliente/serviço com sucesso.
// A rota só ALTERA uma autenticação que já existe — nenhuma rota da API de integração cria a primeira
// (confirmado pelo suporte HubSoft: o campo "parametros" do cadastro é só dos parâmetros do plano e
// grava login/senha ali, sem criar autenticação). O registro só existe se a automação "gerar login
// automaticamente" (máscara de login) estiver ativa na HubSoft: aí o serviço já nasce com um login
// predefinido e esta chamada o substitui pelo login/senha do CSV. Por isso só faz sentido chamar
// depois da criação, nunca antes/junto. idInterfaceConexao é opcional (sem catálogo público para validar o ID
// contra a conta, então é repassado como veio, sem tentar adivinhar ou corrigir).
func ApplyLoginConfig(ctx context.Context, cfg Config, token, idClienteServico, idInterfaceConexao, login, senha string) LoginConfigResult {
	wantLogin := login
	iface := idInterfaceConexao
	res := postLoginConfig(ctx, cfg, token, idClienteServico, iface, login, senha)
	// Com a automação de login automático, o serviço já nasce na interface certa (e, num reenvio, já
	// pode ter o login certo). Pedir de novo o que já está faz a HubSoft recusar o pedido INTEIRO
	// ("já encontra-se vinculado à interface de conexão ... / já encontra-se com o login ..., não é
	// necessário a alteração") — inclusive o que faltava mudar. Esses avisos são inofensivos: refaz o
	// pedido sem o campo que já estava certo; se não sobrou nada a alterar, a linha está OK.
	var notes []string
	for attempt := 0; attempt < 2 && !res.OK; attempt++ {
		switch {
		case iface != "" && isAlreadyLinkedInterfaceMsg(res.Message):
			iface = ""
			notes = append(notes, "interface de conexão já era a informada")
		case login != "" && isAlreadyLoginMsg(res.Message):
			login = ""
			notes = append(notes, "login já era o informado")
		default:
			attempt = 2
			continue
		}
		if login == "" && senha == "" && iface == "" {
			return LoginConfigResult{OK: true, Message: "já estava como informado — nada a alterar (" + strings.Join(notes, "; ") + ")"}
		}
		res = postLoginConfig(ctx, cfg, token, idClienteServico, iface, login, senha)
	}
	if res.OK && len(notes) > 0 {
		res.Message += " (" + strings.Join(notes, "; ") + ")"
	}
	// A HubSoft responde "success" sem garantir que o que pedimos foi aplicado (já aconteceu de trocar o
	// login e manter a senha antiga) — relê o serviço e confere login E senha antes de dar a linha como OK.
	if res.OK && (wantLogin != "" || senha != "") {
		got, gotSenha, found := serviceAuth(ctx, cfg, token, idClienteServico)
		// A HubSoft padroniza o login em minúsculas e o Radius dela não diferencia caixa (confirmado pela HubSoft):
		// diferença SÓ de caixa é aceita — o login original vai para as Observações da autenticação (abaixo).
		caseOnly := found && wantLogin != "" && got != "" && got != wantLogin && strings.EqualFold(got, wantLogin)
		if found && wantLogin != "" && got != "" && got != wantLogin && !caseOnly {
			res.OK = false
			res.Message = fmt.Sprintf("a HubSoft respondeu sucesso, mas o login do serviço continua %q (esperado %q)", got, wantLogin)
		}
		if res.OK && caseOnly {
			if rec := RecordLoginObservation(ctx, cfg, token, idClienteServico, wantLogin); rec != nil {
				if rec.OK {
					res.Message += " (" + rec.Message + ")"
				} else {
					res.OK = false
					res.Message = "login configurado (a HubSoft padronizou em minúsculas), mas não foi possível registrar o login original nas Observações: " + rec.Message
				}
			}
		}
		if res.OK && found && senha != "" && passwordMismatch(gotSenha, senha) {
			// 2ª chamada só com a senha, depois relê de novo — nunca deixa a linha OK com a senha errada.
			fix := postLoginConfig(ctx, cfg, token, idClienteServico, "", "", senha)
			if !fix.OK {
				res.OK = false
				res.Message = "login configurado, mas a senha ficou diferente da informada e a correção falhou: " + fix.Message
			} else if _, again, ok2 := serviceAuth(ctx, cfg, token, idClienteServico); ok2 && passwordMismatch(again, senha) {
				res.OK = false
				res.Message = "login configurado, mas a senha na HubSoft continua diferente da informada mesmo após nova tentativa — corrija manualmente"
			} else {
				res.Message += " (senha reaplicada em 2ª chamada)"
			}
		}
	}
	return res
}

// passwordMismatch — true quando a HubSoft devolveu uma senha legível e ela difere da esperada. Senha
// vazia ou com cara de hash/mascarada não dá para comparar: nesse caso NÃO acusa divergência.
func passwordMismatch(got, want string) bool {
	got = strings.TrimSpace(got)
	if got == "" || got == want || senhaLooksHashed(got) {
		return false
	}
	return true
}

func senhaLooksHashed(s string) bool {
	if strings.HasPrefix(s, "$") || strings.Contains(s, "*") {
		return true
	}
	return len(s) >= 32 && regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`).MatchString(s)
}

// LoginObservationPrefix é o texto que fica nas Observações da autenticação do serviço quando a HubSoft
// padronizou o login em minúsculas: guarda o login ORIGINAL (com a caixa certa) para um futuro ERP/Radius
// que, ao contrário do atual, diferencie maiúsculas de minúsculas.
const LoginObservationPrefix = "Login PPPoE configurado no cliente:"

func loginObservationText(login string) string { return LoginObservationPrefix + " " + login }

// needsLoginObservation — o login da HubSoft difere do original SÓ na caixa (a HubSoft padronizou).
func needsLoginObservation(original, hubsoft string) bool {
	return original != "" && hubsoft != "" && original != hubsoft && strings.EqualFold(original, hubsoft)
}

// mergeObservation acrescenta o texto às observações já existentes (nunca apaga o que o operador escreveu).
// changed=false quando o texto já consta.
func mergeObservation(existing, text string) (merged string, changed bool) {
	existing = strings.TrimSpace(existing)
	if strings.Contains(existing, text) {
		return existing, false
	}
	if existing == "" {
		return text, true
	}
	return existing + "\n" + text, true
}

// RecordLoginObservation grava nas Observações da autenticação do serviço "Login PPPoE configurado no
// cliente: <login original>" — só quando a HubSoft padronizou o login (difere do original apenas na caixa) e
// o texto ainda não está lá. Observações já escritas pelo operador são preservadas (o texto é acrescentado).
// Devolve nil quando não há nada a registrar.
func RecordLoginObservation(ctx context.Context, cfg Config, token, idClienteServico, loginOriginal string) *LoginConfigResult {
	got, _, obs, found := serviceAuthObs(ctx, cfg, token, idClienteServico)
	if !found || !needsLoginObservation(loginOriginal, got) {
		return nil
	}
	text := loginObservationText(loginOriginal)
	merged, changed := mergeObservation(obs, text)
	if !changed {
		return &LoginConfigResult{OK: true, Message: "login original já estava registrado nas Observações"}
	}
	if !isPosInt(idClienteServico) {
		return &LoginConfigResult{Message: fmt.Sprintf("id_cliente_servico inválido (%q)", idClienteServico)}
	}
	fix := postAuthBody(ctx, cfg, token, map[string]any{"id_cliente_servico": atoiSafe(idClienteServico), "observacoes": merged})
	if !fix.OK {
		return &LoginConfigResult{Message: fix.Message}
	}
	if _, _, again, ok := serviceAuthObs(ctx, cfg, token, idClienteServico); ok && !strings.Contains(again, text) {
		return &LoginConfigResult{Message: "a HubSoft respondeu sucesso, mas a observação não ficou gravada"}
	}
	return &LoginConfigResult{OK: true, Message: fmt.Sprintf("login original %q registrado nas Observações da autenticação", loginOriginal)}
}

// RepairPlaceholderPassword — conserta serviço que já tem o login certo mas ficou com a senha padrão da
// máscara (a troca de senha não pegou numa importação anterior). Só age quando a senha lida na HubSoft
// é exatamente PlaceholderPassword: qualquer outra senha pode ser a real do cliente e NUNCA é tocada.
// Devolve nil quando não há nada a fazer.
func RepairPlaceholderPassword(ctx context.Context, cfg Config, token, idClienteServico, login, senha string) *LoginConfigResult {
	if senha == "" || senha == PlaceholderPassword {
		return nil
	}
	got, gotSenha, found := serviceAuth(ctx, cfg, token, idClienteServico)
	if !found || gotSenha != PlaceholderPassword || (login != "" && !strings.EqualFold(got, login)) {
		return nil
	}
	fix := postLoginConfig(ctx, cfg, token, idClienteServico, "", "", senha)
	if !fix.OK {
		return &LoginConfigResult{Message: "serviço com a senha padrão — a correção falhou: " + fix.Message}
	}
	if _, again, ok := serviceAuth(ctx, cfg, token, idClienteServico); ok && passwordMismatch(again, senha) {
		return &LoginConfigResult{Message: "serviço com a senha padrão — a HubSoft respondeu sucesso, mas a senha continua diferente da informada"}
	}
	return &LoginConfigResult{OK: true, Message: "serviço já existia com a senha padrão — senha corrigida"}
}

func isAlreadyLoginMsg(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "já encontra-se com o login")
}

func isAlreadyLinkedInterfaceMsg(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "já encontra-se vinculado") && strings.Contains(m, "interface")
}

// postLoginConfig faz UMA chamada a POST /cliente/configurar_autenticacao (campos vazios não vão no corpo).
func postLoginConfig(ctx context.Context, cfg Config, token, idClienteServico, idInterfaceConexao, login, senha string) LoginConfigResult {
	res := LoginConfigResult{}
	if !isPosInt(idClienteServico) {
		res.Message = fmt.Sprintf("id_cliente_servico inválido (veio %q) — login não configurado", idClienteServico)
		return res
	}
	body := map[string]any{"id_cliente_servico": atoiSafe(idClienteServico)}
	if idInterfaceConexao != "" {
		body["id_interface_conexao"] = atoiSafe(idInterfaceConexao)
	}
	if login != "" {
		body["login"] = login
	}
	if senha != "" {
		body["password"] = senha
	}
	return postAuthBody(ctx, cfg, token, body)
}

// postAuthBody — UMA chamada a POST /cliente/configurar_autenticacao com o corpo já montado.
func postAuthBody(ctx context.Context, cfg Config, token string, body map[string]any) LoginConfigResult {
	res := LoginConfigResult{}
	raw, _ := json.Marshal(body)
	put := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "POST", Path: "/api/v1/integracao/cliente/configurar_autenticacao", BodyTemplate: string(raw), BodyType: "json",
	})
	respBody := ResponseBodyBytes(put)
	if !put.OK {
		if put.StatusCode == 0 {
			res.Message = "sem resposta da HubSoft ao configurar o login — confira manualmente"
			return res
		}
		if msg := hubsoftActionMessageWithErrors(respBody); msg != "" {
			res.Message = withNoAuthHint(msg)
			return res
		}
		res.Message = firstNonEmpty(put.ErrorMessage, fmt.Sprintf("HTTP %d", put.StatusCode))
		return res
	}
	var doc struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
	}
	_ = json.Unmarshal(respBody, &doc)
	if doc.Status != "success" {
		res.Message = withNoAuthHint(firstNonEmpty(hubsoftActionMessageWithErrors(respBody), "a HubSoft não confirmou a configuração do login"))
		return res
	}
	res.OK = true
	res.Message = firstNonEmpty(doc.Msg, "login configurado")
	return res
}

// serviceAuthObs — como serviceAuth, mais as Observações da autenticação.
func serviceAuthObs(ctx context.Context, cfg Config, token, idClienteServico string) (login, senha, obs string, found bool) {
	svcs, err := dvLookup(ctx, cfg, token, "id_cliente_servico", idClienteServico)
	if err != nil {
		return "", "", "", false
	}
	for _, sv := range svcs {
		if sv.ServiceID == idClienteServico {
			return sv.Login, sv.Senha, sv.Obs, true
		}
	}
	return "", "", "", false
}

// serviceAuth relê o serviço na HubSoft e devolve login e senha atuais. found=false quando a leitura
// falha ou o serviço não vem na resposta — nesse caso a conferência é só ignorada, nunca reprova.
func serviceAuth(ctx context.Context, cfg Config, token, idClienteServico string) (login, senha string, found bool) {
	svcs, err := dvLookup(ctx, cfg, token, "id_cliente_servico", idClienteServico)
	if err != nil {
		return "", "", false
	}
	for _, sv := range svcs {
		if sv.ServiceID == idClienteServico {
			return sv.Login, sv.Senha, true
		}
	}
	return "", "", false
}

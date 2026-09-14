// Package panicguard é a rede de segurança para goroutines "detached" — disparadas com
// `go func(){...}()` fora do ciclo de vida de um pedido HTTP, portanto NÃO cobertas pelo
// chi middleware.Recoverer (esse só protege a goroutine que atende cada pedido). Em Go, um
// panic em QUALQUER goroutine sem recover termina o processo inteiro — antes desta rede, um bug
// isolado num ciclo de fundo (ex.: o rodízio telnet de uma OLT específica) conseguia derrubar a
// API e todo o monitoramento juntos, exigindo reinício do contentor.
//
// Uso: `defer panicguard.Recover("nome-descritivo")` como a PRIMEIRA linha dentro de cada
// `go func(){ ... }()` (não muda a assinatura nem a forma como a goroutine é chamada). Um panic
// ali é registado e a goroutine termina — só aquele ciclo fica incompleto; o resto do sistema
// segue, e o próximo tick/ciclo tenta de novo (ver internal/monitorworker/worker.go).
package panicguard

import (
	"log"
	"runtime/debug"
)

// Recover captura um panic na goroutine actual e regista-o em vez de propagar (que mataria o
// processo). Chamar via `defer` logo no início da goroutine a proteger.
func Recover(name string) {
	if r := recover(); r != nil {
		log.Printf("[PANIC RECOVERED] goroutine %q: %v\n%s", name, r, debug.Stack())
	}
}

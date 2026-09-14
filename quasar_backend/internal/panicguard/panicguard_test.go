package panicguard

import "testing"

func TestRecover_stopsPanicFromPropagating(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover("test-goroutine")
		panic("boom — simulated bug in a background cycle")
	}()
	<-done // se Recover não funcionasse, o panic mataria o processo de testes inteiro (t.Fatal nunca rodaria)
}

func TestRecover_noPanicIsNoop(t *testing.T) {
	ran := false
	func() {
		defer Recover("test-noop")
		ran = true
	}()
	if !ran {
		t.Fatal("função protegida não rodou")
	}
}

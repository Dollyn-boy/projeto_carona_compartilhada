package concorrencia

import (
	"testing"
	"time"
)

func TestTrechosDiferentesNaoBloqueiamUmAoOutro(t *testing.T) {
	trava := NovaTrava()

	// Um channel é um mecanismo de comunicação e sincronização entre goroutines.

	entrou := make(chan struct{})  // canal para sinalizar que a primeira operação entrou na seção crítica
	liberar := make(chan struct{}) // canal para sinalizar que a primeira operação pode liberar a seção crítica

	// 1. Inicia uma goroutine que adquire a trava para o trecho "carona-1:0" e espera até receber um sinal para liberar.
	go func() {
		// A função anônima começa a executar concorrentemente com o próprio teste.
		// Isso permite simular duas operações que tentam acessar recursos do sistema ao mesmo tempo.
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			// Sinaliza que entrou na seção crítica e espera até receber um sinal para liberar.
			close(entrou)
			// Espera até receber um sinal para liberar a seção crítica.
			<-liberar
			return nil
		})
	}()

	select {
	case <-entrou:
	case <-time.After(time.Second):
		t.Fatal("a primeira operacao nao adquiriu a trava")
	}

	terminou := make(chan struct{})
	go func() {
		_ = trava.ComTrechos([]string{"carona-1:1"}, func() error {
			close(terminou)
			return nil
		})
	}()

	select {
	case <-terminou:
		// O segundo trecho deve prosseguir sem esperar pelo primeiro.
	case <-time.After(time.Second):
		close(liberar)
		t.Fatal("trechos diferentes ficaram bloqueados entre si")
	}
	close(liberar)
}

func TestMesmoTrechoExigeTravaExclusiva(t *testing.T) {
	trava := NovaTrava()
	entrou := make(chan struct{})
	liberar := make(chan struct{})
	terminouSegundo := make(chan struct{})

	go func() {
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			close(entrou)
			<-liberar
			return nil
		})
	}()
	<-entrou

	go func() {
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			close(terminouSegundo)
			return nil
		})
	}()

	select {
	case <-terminouSegundo:
		t.Fatal("duas operacoes entraram simultaneamente no mesmo trecho")
	case <-time.After(40 * time.Millisecond):
	}

	close(liberar)
	select {
	case <-terminouSegundo:
	case <-time.After(time.Second):
		t.Fatal("a segunda operacao nao prosseguiu apos a liberacao da trava")
	}
}

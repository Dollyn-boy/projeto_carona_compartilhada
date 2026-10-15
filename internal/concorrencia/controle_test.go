package concorrencia

import (
	"testing"
	"time"
)

func TestTrechosDiferentesNaoBloqueiamUmAoOutro(t *testing.T) {
	trava := NovaTrava()
	entrou := make(chan struct{})
	liberar := make(chan struct{})

	go func() {
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			close(entrou)
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

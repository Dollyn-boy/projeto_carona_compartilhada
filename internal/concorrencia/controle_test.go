package concorrencia

import (
	"testing"
	"time"
)

func TestDuasOperacoesNoMesmoTrechoNaoPodemExecutarSimultaneamente(t *testing.T) {
	trava := NovaTrava() // Gerenciador de travas para trechos
	// Use ComTrechos() com a mesma chave em duas goroutines.
	// Mantenha a primeira bloqueada e comprove que a segunda não entra na seção crítica

	primeiraEntrou := make(chan struct{})
	liberarPrimeira := make(chan struct{})
	segundaEntrou := make(chan struct{})

	// Adquire a trava compartilhada e é bloqueada até a segunda liberar
	go func() {
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			close(primeiraEntrou)
			<-liberarPrimeira
			return nil
		})
	}()

	select {
	case <-primeiraEntrou: // Aguarda primeira goroutine entrar na sessão crítica para continuar teste
	case <-time.After(time.Second):
		t.Fatal("a primeira goroutine não entrou")
	}

	// Após a primeira goroutine atingir a sessão crítica
	// Tentar adquirir a mesma trava
	go func() {
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			// A segunda só atingirá a região crítica se conseguir acessar a trava do trecho
			close(segundaEntrou)
			return nil
		})
	}()

	select {
	case <-segundaEntrou:
		close(liberarPrimeira)
		t.Fatal("a segunda goroutine entrou antes da liberação")
	case <-time.After(100 * time.Millisecond):
		// Esperado: a segunda continua bloqueada.
	}

	// Libera a primeira para que a segunda possa prosseguir.
	close(liberarPrimeira)

	select {
	case <-segundaEntrou:
		// Correto: a segunda entrou depois da liberação.
	case <-time.After(time.Second):
		t.Fatal("a segunda goroutine não entrou após a liberação")
	}

}

func TestTrechosDiferentesNaoBloqueiamUmAoOutro(t *testing.T) {
	trava := NovaTrava() // Gerenciador de travas para trechos, garantindo que operações em trechos diferentes não bloqueiem umas às outras.

	// Um channel é um mecanismo de comunicação e sincronização entre goroutines.

	entrou := make(chan struct{})  // canal para sinalizar que a primeira operação entrou na seção crítica
	liberar := make(chan struct{}) // canal para sinalizar que a primeira operação pode liberar a seção crítica

	// Inicia a primeira operação em uma goroutine separada. Ela tentará adquirir a trava para o trecho "carona-1:0".
	go func() {
		// A função anônima começa a executar concorrentemente com o próprio teste.
		// Isso permite simular duas operações que tentam acessar recursos do sistema ao mesmo tempo.

		// ComTrechos adquire a trava para o trecho "carona-1:0" e executa a função fornecida enquanto a trava estiver adquirida.
		_ = trava.ComTrechos([]string{"carona-1:0"}, func() error {
			// Sinaliza que entrou na seção crítica e espera até receber um sinal para liberar.
			close(entrou) // fecha o canal entrou para sinalizar que a primeira operação entrou na seção crítica
			// Go routine espera até receber um sinal para liberar a seção crítica.
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
		close(liberar) // Isso desbloqueia as goroutines que estiverem esperando para receber de liberar.
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

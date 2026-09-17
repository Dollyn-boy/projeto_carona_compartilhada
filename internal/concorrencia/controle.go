
package concorrencia

import "sync"

// Trava implementa o controle de concorrência mais simples possível:
// uma única trava global, compartilhada por TODAS as operações, não
// importa quais trechos estejam envolvidos.

type Trava struct {
	mu sync.RWMutex
}

// NovaTrava cria uma trava global pronta para uso.
func NovaTrava() *Trava {
	return &Trava{}
}

// ComEscrita executa fn com acesso EXCLUSIVO: nenhuma outra chamada a
// ComEscrita ou ComLeitura roda ao mesmo tempo. Use para qualquer
// operação que MODIFICA o estado: PublicarCarona, ConfirmarReserva,
// CancelarCarona, CancelarReserva.
func (t *Trava) ComEscrita(fn func() error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn()
}

// ComLeitura executa fn com acesso COMPARTILHADO: outras chamadas a
// ComLeitura podem rodar ao mesmo tempo, mas nenhuma ComEscrita roda
// enquanto isto não terminar. Use para operações que só LEEM o estado:
// BuscarItinerarios, ConsultarCaronas, ConsultarReservas.
func (t *Trava) ComLeitura(fn func() error) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return fn()
}

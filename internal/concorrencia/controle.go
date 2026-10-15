// Package concorrencia fornece travas por recurso (neste projeto, por
// trecho de uma carona). Operacoes que precisam reservar varios trechos
// adquirem as travas em ordem alfabetica para evitar deadlocks.
package concorrencia

import (
	"sort"
	"strconv"
	"sync"
)

type entradaTrava struct {
	mu          sync.Mutex
	referencias int
}

// Trava administra uma trava independente para cada chave de trecho.
// O mapa interno so serve para localizar a trava certa; ele nao protege
// os dados do repositorio (isso e responsabilidade de estado.Repositorio).
type Trava struct {
	mu     sync.Mutex
	travas map[string]*entradaTrava
}

// NovaTrava cria um gerenciador de travas por trecho.
func NovaTrava() *Trava {
	return &Trava{travas: make(map[string]*entradaTrava)}
}

// ChaveTrecho cria uma chave unica para um trecho elementar de uma carona.
func ChaveTrecho(idCarona string, indice int) string {
	return idCarona + ":" + strconv.Itoa(indice)
}

// ComTrechos executa fn enquanto possui as travas de todos os trechos
// informados. Chaves repetidas sao removidas e a ordem e sempre a mesma,
// independente da ordem recebida pelo chamador.
func (t *Trava) ComTrechos(chaves []string, fn func() error) error {
	if len(chaves) == 0 {
		return fn()
	}

	// Copiamos antes de ordenar para nao alterar a slice do chamador.
	ordenadas := append([]string(nil), chaves...)
	sort.Strings(ordenadas)
	unicas := ordenadas[:0]
	for _, chave := range ordenadas {
		if len(unicas) == 0 || unicas[len(unicas)-1] != chave {
			unicas = append(unicas, chave)
		}
	}

	// Registra as referencias antes de esperar pelas travas. Assim, uma
	// trava nao pode ser removida do mapa enquanto outra goroutine espera nela.
	entradas := make([]*entradaTrava, len(unicas))
	t.mu.Lock()
	for i, chave := range unicas {
		entrada, existe := t.travas[chave]
		if !existe {
			entrada = &entradaTrava{}
			t.travas[chave] = entrada
		}
		entrada.referencias++
		entradas[i] = entrada
	}
	t.mu.Unlock()

	for _, entrada := range entradas {
		entrada.mu.Lock()
	}
	defer func() {
		// Libera em ordem inversa, como numa pilha.
		for i := len(entradas) - 1; i >= 0; i-- {
			entradas[i].mu.Unlock()
		}

		t.mu.Lock()
		for i, chave := range unicas {
			entradas[i].referencias--
			if entradas[i].referencias == 0 {
				delete(t.travas, chave)
			}
		}
		t.mu.Unlock()
	}()

	return fn()
}

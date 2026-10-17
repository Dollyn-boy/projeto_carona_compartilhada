// Package concorrencia fornece travas por recurso (neste projeto, por
// trecho de uma carona). Operacoes que precisam reservar varios trechos
// adquirem as travas em ordem alfabetica para evitar deadlocks.
package concorrencia

// travas["1:0"] = &entradaTrava{mu: ..., referencias: 2}
//  ├── "1:0" → trava do trecho A-B
//  ├── "1:1" → trava do trecho B-C
//  └── "2:0" → trava do primeiro trecho de outra carona

import (
	"sort"
	"strconv"
	"sync"
)

// entradaTrava representa uma trava de um trecho. Ela possui um mutex para
// proteger o acesso a ela e um contador de referencias para saber quantas
// goroutines estao usando a trava. Quando o contador chega a zero, a trava
// pode ser removida do mapa de travas.
type entradaTrava struct {
	mu          sync.Mutex //	Impede que duas operações protegidas pela mesma chave executem simultaneamente.
	referencias int        // Conta quantas operações estão usando ou aguardando aquela entrada de trava.
}

// Trava e um gerenciador de travas por trecho.
// Protege o mapa que relaciona uma chave, como "7:0", ao objeto de trava daquele trecho.
// Ele não protege diretamente os mapas de caronas, assentos ou reservas.
type Trava struct {
	mu     sync.Mutex               // Protege o mapa que associa cada chave à sua trava.
	travas map[string]*entradaTrava // Mapa referenciando cada trava por chave de trecho.
}

// NovaTrava cria um gerenciador de travas por trecho.
func NovaTrava() *Trava {
	return &Trava{travas: make(map[string]*entradaTrava)}
}

// ChaveTrecho cria uma chave unica para um trecho elementar de uma carona.
// A chave e composta pelo id da carona e o indice do trecho na rota.
func ChaveTrecho(idCarona string, indice int) string {
	return idCarona + ":" + strconv.Itoa(indice)
}

// Recebe as chaves dos trechos que precisam ser protegidos, adquira suas travas,
// execute fn() enquanto elas estiverem adquiridas e, ao terminar, libere as travas.
func (t *Trava) ComTrechos(chaves []string, fn func() error) error {
	// Se não houver chaves, apenas executa a função sem travas.
	if len(chaves) == 0 {
		return fn()
	}

	// Copiamos antes de ordenar para nao alterar a slice do chamador.
	ordenadas := append([]string(nil), chaves...)
	sort.Strings(ordenadas)

	// Remove chaves duplicadas, pois nao faz sentido travar o mesmo trecho duas vezes.
	unicas := ordenadas[:0]
	for _, chave := range ordenadas {
		if len(unicas) == 0 || unicas[len(unicas)-1] != chave {
			unicas = append(unicas, chave)
		}
	}

	// Cria uma entrada de trava para cada chave unica, incrementando o contador de referencias.
	entradas := make([]*entradaTrava, len(unicas))

	// Bloqueia o mutex do gerenciador de travas para acessar o mapa de travas.
	t.mu.Lock()

	// Para cada chave unica, verifica se ja existe uma entrada de trava no mapa.
	for i, chave := range unicas {
		entrada, existe := t.travas[chave]
		if !existe {
			// Se nao existir, cria uma nova entrada de trava e adiciona ao mapa.
			entrada = &entradaTrava{}
			t.travas[chave] = entrada
		}
		// Incrementa o contador de referencias da entrada de trava, pois uma nova operação vai usá-la.
		entrada.referencias++
		// Armazena a entrada de trava na slice de entradas para poder travá-la depois.
		entradas[i] = entrada
	}

	// Libera o mutex do gerenciador de travas, pois ja temos todas as entradas de trava que precisamos.
	t.mu.Unlock()

	// Trava cada entrada de trava na ordem das chaves unicas.
	for _, entrada := range entradas {
		entrada.mu.Lock()
	}

	// Executa a função passada como argumento enquanto as travas estão adquiridas. A função pode acessar os trechos protegidos com segurança.
	// Ao terminar, libera as travas em ordem inversa e decrementa o contador de referencias de cada entrada de trava.
	// Se o contador chegar a zero, remove a entrada de trava do mapa do gerenciador de travas.
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

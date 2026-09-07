// Package concorrencia é o MECANISMO de controle de concorrência em si
// (as travas/versionamento) — a segunda metade da camada "Dominio +
// concorrencia" da arquitetura. Quem de fato o usa para proteger os
// dados reais é o internal/estado; este pacote não guarda nenhum dado
// de carona/trecho/reserva, só a lógica de exclusão mútua.
package concorrencia

import "sync"

// Trava implementa o controle de concorrência mais simples possível:
// uma única trava global, compartilhada por TODAS as operações, não
// importa quais trechos estejam envolvidos.
//
// Por que sync.RWMutex e não um sync.Mutex comum: leituras
// (BuscarItinerarios, ConsultarCaronas, ConsultarReservas) podem rodar
// em paralelo entre si sem risco nenhum, desde que NENHUMA escrita
// esteja acontecendo ao mesmo tempo. RLock() permite exatamente isso —
// várias leituras simultâneas — mas qualquer Lock() (escrita) espera
// todas as leituras em andamento terminarem, e bloqueia leituras novas
// até a escrita acabar. Ainda é uma trava GLOBAL (não distingue quais
// trechos estão envolvidos), só diferencia leitura de escrita.
//
// Vantagem: trivialmente correta — só existe UMA trava, então não há
// como pedir em ordens diferentes (deadlock impossível por construção).
// Custo: toda ESCRITA ainda serializa com qualquer outra escrita, mesmo
// que envolvam trechos completamente diferentes (duas confirmações de
// reserva em caronas sem nenhuma relação uma com a outra esperam a
// mesma fila). Se isso virar gargalo no teste de carga, a evolução
// natural é trocar por travas por trecho — ver as opções documentadas
// no fim deste arquivo.
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

// ============================================================================
// TODO — se um dia esta trava global virar gargalo no teste de carga
// ============================================================================
//
// As duas opções abaixo são mais granulares (deixam ESCRITAS que não
// disputam os mesmos trechos rodarem em paralelo de verdade), ao custo
// de mais complexidade de implementação:
//
//   OPÇÃO 2 — travas por chave, com ORDEM CANÔNICA:
//     um mutex por chave de trecho (ex: "idCarona:indice"). Para travar
//     N chaves de uma vez: ORDENE as chaves antes de adquirir qualquer
//     mutex, e sempre adquira nessa mesma ordem — é isso que evita
//     deadlock (o mesmo raciocínio do clássico "jantar dos filósofos").
//
//   OPÇÃO 3 — versionamento otimista (compare-and-swap):
//     cada contador de trecho carrega uma versão; ConfirmarReserva lê
//     um snapshot sem travar nada, tenta aplicar via CAS, tenta de novo
//     se algo mudou no meio.

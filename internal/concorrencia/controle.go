// Package concorrencia e a camada que garante que a confirmacao de uma
// reserva sobre um ou mais trechos (possivelmente de caronas
// diferentes) seja atomica e livre de condicoes de corrida — sem
// delegar isso a nenhum banco de dados ou servico externo de
// coordenacao (proibido pelo enunciado).
package concorrencia

// TODO — este arquivo deve conter o mecanismo de controle de concorrencia.
//
// Escolha UMA abordagem (documente a escolha no relatorio):
//   1. Trava global unica sobre todo o estado — simples e correta, mas
//      serializa todas as confirmacoes.
//   2. Travas por trecho, adquiridas SEMPRE na mesma ordem canonica
//      (ex: ordenando por idCarona+indice do trecho) — evita deadlock e
//      permite mais concorrencia real.
//   3. Versionamento otimista (compare-and-swap) por trecho, com nova
//      tentativa em caso de conflito.
//
// Regra que nao pode ser violada: a secao critica de confirmacao nunca
// pode esperar por I/O de rede no meio — o cliente ja deve enviar o
// itinerario completo numa unica requisicao. Isso e o que impede um
// assento de ficar bloqueado para sempre por uma reserva nunca concluida.
//
// Valide esta camada com goroutines concorrentes ANTES de conectar
// qualquer rede — e sempre rode os testes com: go test -race

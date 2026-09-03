// Package concorrencia e o MECANISMO de controle de concorrencia em si
// (as travas/versionamento) — a segunda metade da camada "Dominio +
// concorrencia" da arquitetura. Quem de fato o usa para proteger os
// dados reais e o internal/estado; este pacote nao guarda nenhum dado
// de carona/trecho/reserva, so a logica de exclusao mutua.
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

package dominio

// TODO — este arquivo deve conter a estrutura de disponibilidade por trecho.
//
// Para uma carona com rota [A, B, C, D], existem 3 trechos elementares:
// A-B, B-C, C-D. Cada um tem seu proprio contador de assentos disponiveis.
//
// Um passageiro que reserva de A ate C ocupa os trechos A-B e B-C
// simultaneamente. Pense nisso como um vetor de contadores por carona,
// onde reservar um trajeto [i,j] exige checar (e depois decrementar)
// todos os contadores entre i e j.
//
// Esta e a estrutura de dados que o pacote concorrencia vai proteger —
// pense em como identificar um trecho de forma unica (ex: idCarona +
// indice do trecho) para permitir travas ordenadas mais tarde.

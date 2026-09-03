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
// O contador em si — o dado mutavel de fato — e guardado e protegido em
// internal/estado. Aqui ficam so o tipo e as funcoes puras que operam
// sobre um snapshot desses contadores (ex: "este trajeto cabe nesses
// contadores?").

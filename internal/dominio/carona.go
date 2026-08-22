// Package dominio contem o modelo de dados central do VaiJunto: caronas,
// trechos e reservas — e a logica de negocio pura, sem nenhuma
// dependencia de rede ou protocolo. Deve ser inteiramente testavel sem
// subir nenhum socket.
package dominio

// TODO — este arquivo deve conter o tipo Carona.
//
// Uma carona e publicada por um motorista e tem:
//   - um identificador unico;
//   - o motorista responsavel;
//   - uma rota: sequencia ORDENADA de cidades (ex: A, B, C, D);
//   - data e horario de partida;
//   - preco por trecho elementar (pode variar por trecho, ou ser fixo — decida);
//   - a capacidade de assentos do veiculo.
//
// Lembrete: a disponibilidade de assentos NAO e por carona inteira, e por
// trecho elementar da rota (ver trecho.go). Um assento ocupado entre a
// primeira e a segunda cidade continua livre para os trechos seguintes.

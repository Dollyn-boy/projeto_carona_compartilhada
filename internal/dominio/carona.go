// Package dominio contem os TIPOS e as regras de negocio PURAS do
// VaiJunto — caronas, trechos e reservas — sem nenhuma dependencia de
// rede, protocolo ou estado mutavel compartilhado. Deve ser inteiramente
// testavel sem subir nenhum socket. Junto com internal/concorrencia,
// forma a camada "Dominio + concorrencia" da arquitetura; o
// armazenamento de fato desses dados em memoria vive em internal/estado.
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

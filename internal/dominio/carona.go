// Package dominio contem os TIPOS e as regras de negocio PURAS do
// VaiJunto — caronas, trechos e reservas — sem nenhuma dependencia de
// rede, protocolo ou estado mutavel compartilhado. Deve ser inteiramente
// testavel sem subir nenhum socket. Junto com internal/concorrencia,
// forma a camada "Dominio + concorrencia" da arquitetura; o
// armazenamento de fato desses dados em memoria vive em internal/estado.
package dominio

// Carona e publicada por um motorista e tem:
//   - Id: identificador unico;
//   - Motorista: o responsavel pela carona;
//   - Rota: sequencia ORDENADA de cidades (ex: A, B, C, D);
//   - Preco: fixo para a carona inteira nesta versao (decisao de
//     simplificacao — poderia variar por trecho, mas aqui e um so valor
//     para todos os trechos);
//   - Capacidade: assentos do veiculo, mesmo numero para todo trecho.
//
// CORRECAO: Rota e Capacidade precisam ser EXPORTADOS (letra maiuscula)
// — campos comecando com minuscula so sao visiveis dentro do proprio
// pacote "dominio". Como internal/estado e um pacote DIFERENTE, ele
// nunca conseguiria ler nem escrever "rota"/"capcidade" do jeito que
// estava (nem compilaria). Tambem corrigi o typo capcidade -> Capacidade.
//
// NOTA (gap conhecido, nao bloqueante): ainda nao ha campo de data aqui.
// internal/estado.BuscarItinerarios por enquanto busca em TODAS as
// caronas publicadas, sem filtrar por data — adicione um campo (ex:
// Data time.Time) quando decidir o formato, e ajuste BuscarItinerarios
// para filtrar por ele.
//
// Lembrete: a disponibilidade de assentos NAO e por carona inteira, e por
// trecho elementar da rota (ver trecho.go). Um assento ocupado entre a
// primeira e a segunda cidade continua livre para os trechos seguintes.
type Carona struct {
	Id         int
	Motorista  string
	Rota       []Cidade
	Preco      int
	Capacidade int
}

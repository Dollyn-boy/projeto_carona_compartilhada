package dominio

type Cidade string

// Trecho representa um trecho ELEMENTAR de uma carona: o intervalo entre
// duas cidades ADJACENTES na rota de uma carona especifica. Para uma
// carona com rota [A, B, C, D], existem 3 trechos elementares: A-B
// (Indice 0), B-C (Indice 1), C-D (Indice 2).
//

type Trecho struct {
	IDCarona string
	Indice   int // posicao do trecho na rota: 0 = primeiro trecho da carona
	Origem   Cidade
	Destino  Cidade
}

// TrechosDaRota constroi a lista de trechos elementares a partir de uma
// rota ordenada de cidades. Ex: TrechosDaRota("carona-1", []Cidade{"A",
// "B", "C", "D"}) devolve os trechos A-B, B-C, C-D nessa ordem — os
// mesmos 3 trechos do exemplo acima.
//
// Isso e usado tanto na hora de publicar uma carona (para saber quantos
// contadores de assento criar em internal/estado) quanto na hora de
// montar o grafo de busca (ver busca.go).
func TrechosDaRota(idCarona string, rota []Cidade) []Trecho {
	// Uma rota com N cidades tem N-1 trechos elementares. Pre-alocar a
	// slice com essa capacidade evita realocacoes durante o append.
	trechos := make([]Trecho, 0, len(rota)-1)

	for i := 0; i < len(rota)-1; i++ {
		trechos = append(trechos, Trecho{
			IDCarona: idCarona,
			Indice:   i,
			Origem:   rota[i],
			Destino:  rota[i+1],
		})
	}

	return trechos
}

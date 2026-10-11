package dominio

type Cidade string

// Trecho representa um trecho ELEMENTAR de uma carona: o intervalo entre
// duas cidades ADJACENTES na rota de uma carona especifica. Para uma
// carona com rota [A, B, C, D], existem 3 trechos elementares: A-B
// (Indice 0), B-C (Indice 1), C-D (Indice 2).
//

type Trecho struct {
	Empresa  string
	IDCarona string
	Indice   int
	Origem   Cidade
	Destino  Cidade
}

func TrechosDaRota(empresa, idCarona string, rota []Cidade) []Trecho {
	// Uma rota com N cidades tem N-1 trechos elementares. Pre-alocar a
	// slice com essa capacidade evita realocacoes durante o append.
	trechos := make([]Trecho, 0, len(rota)-1)

	for i := 0; i < len(rota)-1; i++ {
		trechos = append(trechos, Trecho{
			Empresa:  empresa,
			IDCarona: idCarona,
			Indice:   i,
			Origem:   rota[i],
			Destino:  rota[i+1],
		})
	}

	return trechos
}

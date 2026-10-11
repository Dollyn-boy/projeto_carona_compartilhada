package dominio

// Este arquivo modela o grafo de busca de itinerarios: cidades sao nos,
// trechos com assento disponivel (numa carona, numa data) sao arestas.

// ArestaDisponivel e uma aresta do grafo de busca: um trecho concreto,
// de uma carona especifica, que ainda tem assento disponivel numa data.
type ArestaDisponivel struct {
	Trecho              Trecho
	AssentosDisponiveis int
	Preco               float64
}

// GrafoItinerarios e a lista de adjacencia do grafo: para cada cidade de
// origem, quais arestas (trechos disponiveis) saem dela. Mapa de slice
// e a forma idiomatica de representar lista de adjacencia em Go.
type GrafoItinerarios map[Cidade][]ArestaDisponivel

// ConstruirGrafo monta o grafo de busca a partir de um SNAPSHOT de
// arestas ja disponiveis (filtradas por data). Quem monta esse
// snapshot e o internal/estado, que sabe o contador de assentos real,
// protegido por internal/concorrencia — esta funcao e pura, so organiza
// os dados que recebe.
func ConstruirGrafo(disponiveis []ArestaDisponivel) GrafoItinerarios {
	grafo := make(GrafoItinerarios)

	for _, aresta := range disponiveis {
		origem := aresta.Trecho.Origem
		grafo[origem] = append(grafo[origem], aresta)
	}

	return grafo
}

// Itinerario e um possivel trajeto encontrado: uma sequencia ordenada de
// trechos — possivelmente de caronas diferentes — que leva da origem ao
// destino pedidos. Este e o tipo de RESULTADO que a busca (ainda TODO
// abaixo) deve devolver.
type Itinerario struct {
	Passos []Trecho
}

func BuscarTodosItinerarios(grafo GrafoItinerarios, origem, destino Cidade) []Itinerario {
	itinerarios := []Itinerario{}

	// O conjunto de visitadas do caminho atual evita ciclos sem impedir que
	// uma cidade seja usada em caminhos diferentes.
	var dfs func(Cidade, []Trecho, map[Cidade]bool)
	dfs = func(atual Cidade, passos []Trecho, visitadas map[Cidade]bool) {
		if atual == destino {
			copia := append([]Trecho(nil), passos...)
			itinerarios = append(itinerarios, Itinerario{Passos: copia})
			return
		}

		for _, aresta := range grafo[atual] {
			proxima := aresta.Trecho.Destino
			if visitadas[proxima] {
				continue
			}

			novosPassos := append(append([]Trecho(nil), passos...), aresta.Trecho)
			novasVisitadas := make(map[Cidade]bool, len(visitadas)+1)
			for cidade := range visitadas {
				novasVisitadas[cidade] = true
			}
			novasVisitadas[proxima] = true
			dfs(proxima, novosPassos, novasVisitadas)
		}
	}

	dfs(origem, nil, map[Cidade]bool{origem: true})
	return itinerarios
}

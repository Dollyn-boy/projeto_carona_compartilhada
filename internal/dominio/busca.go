package dominio

// ArestaDisponivel e uma aresta do grafo de busca: um trecho concreto,
// de uma carona especifica, que ainda tem assento disponivel numa data.
type ArestaDisponivel struct {
	Trecho              Trecho
	AssentosDisponiveis int
	Preco               float64
}

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

// TODO — implemente aqui a busca de fato sobre GrafoItinerarios, algo como:
//
//	func BuscarItinerarios(grafo GrafoItinerarios, origem, destino Cidade) []Itinerario
// func BuscarItinerarios(grafo GrafoItinerarios, origem, destino Cidade) []Itinerario {
// }

// Pontos para decidir:
//   - BFS/DFS ja resolve, dado o volume esperado do projeto;
//   - o enunciado pede "os itinerarios possiveis" no plural — pense se
//     voce quer devolver so o melhor por algum criterio (mais curto,
//     mais barato) ou varias opcoes para o passageiro escolher;
//   - evite ciclos (nao revisitar uma cidade no mesmo itinerario) e
//     considere limitar o numero de trocas de carona, para nao deixar a
//     busca crescer sem controle em grafos mais densos.

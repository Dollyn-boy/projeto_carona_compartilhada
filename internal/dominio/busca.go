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

// TODO(implementado com BFS) — se no futuro voce quiser devolver VARIOS
// itinerarios (nao so o de menos trechos), veja a nota no fim da funcao.
//
// BuscarItinerarios encontra o itinerario com o MENOR NUMERO DE TRECHOS
// entre origem e destino, usando busca em largura (BFS).
func BuscarItinerarios(grafo GrafoItinerarios, origem, destino Cidade) []Itinerario {
	itinerarios := []Itinerario{}

	// Caso trivial: origem e destino iguais, nao ha trecho nenhum pra
	// percorrer.
	if origem == destino {
		return itinerarios
	}

	// visitadas evita revisitar uma cidade (e portanto evita ciclos
	// infinitos). veioDe guarda, para cada cidade ja alcancada, QUAL
	// TRECHO foi usado para chegar nela — e o que permite reconstruir o
	// caminho inteiro no final, andando de tras para frente a partir do
	// destino.
	//
	visitadas := map[Cidade]bool{origem: true}
	veioDe := map[Cidade]Trecho{}

	// fila e o coracao do BFS: um slice usado como fila (FIFO) — o
	// primeiro elemento inserido e o primeiro a ser removido. Go nao tem
	// um tipo fila nativo; fila[0] + fila = fila[1:] simula isso.
	fila := []Cidade{origem}
	encontrouDestino := false

	for len(fila) > 0 && !encontrouDestino {
		atual := fila[0]
		fila = fila[1:]

		for _, aresta := range grafo[atual] {
			proxima := aresta.Trecho.Destino

			// Aqui q eu resolvo o problema de priorizar mesma carona
			if visitadas[proxima] {
				continue // ja alcancada por um caminho igual ou mais curto — ignora
			}

			visitadas[proxima] = true
			veioDe[proxima] = aresta.Trecho

			if proxima == destino {
				encontrouDestino = true
				break // achou o destino: nao precisa expandir mais nada deste nivel
			}

			fila = append(fila, proxima)
		}
	}

	if !visitadas[destino] {
		return itinerarios // destino inalcancavel a partir da origem
	}

	// Reconstrucao do caminho: comecando do destino, seguimos veioDe para
	// tras (Trecho.Origem de cada trecho) ate voltar na origem, guardando
	// os trechos na ordem inversa da viagem.
	var passos []Trecho
	cidadeAtual := destino
	for cidadeAtual != origem {
		trecho := veioDe[cidadeAtual]
		passos = append(passos, trecho)
		cidadeAtual = trecho.Origem
	}

	// passos foi montado de destino -> origem; precisa inverter para
	// ficar na ordem certa de viagem (origem -> destino).
	for i, j := 0, len(passos)-1; i < j; i, j = i+1, j-1 {
		passos[i], passos[j] = passos[j], passos[i]
	}

	itinerarios = append(itinerarios, Itinerario{Passos: passos})
	return itinerarios

	// NOTA sobre "os itinerarios possiveis" no plural: este BFS devolve
	// SO o itinerario de menos trechos (nunca mais de um). Se decidir
	// que o passageiro deve ver varias opcoes para escolher, a forma
	// mais simples de estender isso e trocar "visitadas" por um limite
	// de profundidade (como fizemos na versao com DFS) e enumerar todos
	// os caminhos simples ate esse limite, em vez de parar no primeiro
	// encontrado.
}

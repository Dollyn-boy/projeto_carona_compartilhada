package dominio

import "testing"

// Este arquivo testa ConstruirGrafo e BuscarItinerarios ISOLADAMENTE —
// sem rede, sem internal/estado, sem internal/concorrencia. Cada teste
// abaixo foi escrito pra provar UM comportamento especifico do BFS, na
// ordem que faz mais sentido pra entender o algoritmo de cima a baixo:
// primeiro o caso mais simples, depois multigrafo, depois os casos de
// borda (ciclos, inalcancavel, destino sem saida) que foram justamente
// os pontos que geraram bugs no rascunho original.
//
// Se voce for adicionar uma restricao nova (limite de trocas, preco
// maximo, horario de conexao, etc.), rode esta suite ANTES e DEPOIS da
// mudanca — os testes que continuarem passando confirmam o que nao
// mudou de comportamento; os que quebrarem mostram exatamente onde a
// nova regra precisa entrar.

// aresta e um helper so pra deixar os testes abaixo mais curtos de ler.
func aresta(idCarona string, indice int, origem, destino Cidade, assentos int, preco float64) ArestaDisponivel {
	return ArestaDisponivel{
		Trecho: Trecho{
			IDCarona: idCarona,
			Indice:   indice,
			Origem:   origem,
			Destino:  destino,
		},
		AssentosDisponiveis: assentos,
		Preco:               preco,
	}
}

// --- Casos basicos ---------------------------------------------------

func TestBuscarItinerarios_CaminhoDireto(t *testing.T) {
	// O cenario mais simples possivel: uma unica carona, um unico
	// trecho, ligando origem e destino diretamente.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "B", 2, 30),
	})

	resultado := BuscarItinerarios(grafo, "A", "B")

	if len(resultado) != 1 {
		t.Fatalf("esperava 1 itinerario, veio %d", len(resultado))
	}
	if len(resultado[0].Passos) != 1 || resultado[0].Passos[0].IDCarona != "c1" {
		t.Fatalf("itinerario inesperado: %+v", resultado[0].Passos)
	}
}

func TestBuscarItinerarios_MesmaCarona_VariosTrechos(t *testing.T) {
	// Uma unica carona com rota A -> B -> C -> D (3 trechos elementares,
	// o mesmo exemplo do enunciado). BuscarItinerarios precisa encadear
	// os 3, todos pertencentes a mesma carona.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "B", 2, 10),
		aresta("c1", 1, "B", "C", 2, 10),
		aresta("c1", 2, "C", "D", 2, 10),
	})

	resultado := BuscarItinerarios(grafo, "A", "D")

	if len(resultado) != 1 {
		t.Fatalf("esperava 1 itinerario, veio %d", len(resultado))
	}
	passos := resultado[0].Passos
	if len(passos) != 3 {
		t.Fatalf("esperava 3 trechos, veio %d: %+v", len(passos), passos)
	}
	for i, p := range passos {
		if p.IDCarona != "c1" || p.Indice != i {
			t.Fatalf("trecho %d fora de ordem/carona: %+v", i, p)
		}
	}
}

// --- O multigrafo: o coracao da modelagem -----------------------------

func TestBuscarItinerarios_CombinaCaronasDiferentes(t *testing.T) {
	// A carona "c1" so vai de A ate B. A carona "c2" so vai de B ate C.
	// Nenhuma das duas faz o percurso completo sozinha — a busca precisa
	// COMBINAR as duas. Este e o cenario do proprio enunciado (Salvador
	// -> Feira -> Vitoria da Conquista com motoristas diferentes).
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "B", 2, 20),
		aresta("c2", 0, "B", "C", 1, 25),
	})

	resultado := BuscarItinerarios(grafo, "A", "C")

	if len(resultado) != 1 || len(resultado[0].Passos) != 2 {
		t.Fatalf("esperava 1 itinerario com 2 trechos, veio %+v", resultado)
	}
	if resultado[0].Passos[0].IDCarona != "c1" || resultado[0].Passos[1].IDCarona != "c2" {
		t.Fatalf("nao combinou as caronas certas: %+v", resultado[0].Passos)
	}
}

func TestBuscarItinerarios_Multigrafo_EscolheAPrimeiraArestaInserida(t *testing.T) {
	// Duas caronas DIFERENTES oferecem o MESMO par A->B — isso e o
	// multigrafo em si. O BFS atual NAO escolhe pelo criterio de preco
	// nem de assentos disponiveis: ele so pega a primeira aresta que
	// encontra, na ordem em que foi inserida em ConstruirGrafo (ou seja,
	// a ordem do slice "disponiveis" recebido).
	//
	// Isso e exatamente o ponto que uma futura restricao de "priorizar a
	// mais barata" precisaria mudar — hoje esse criterio nao existe.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("caro", 0, "A", "B", 2, 100),  // inserida primeiro, mais cara
		aresta("barato", 0, "A", "B", 1, 10), // inserida depois, mais barata
	})

	resultado := BuscarItinerarios(grafo, "A", "B")

	if len(resultado) != 1 {
		t.Fatalf("esperava 1 itinerario, veio %d", len(resultado))
	}
	if resultado[0].Passos[0].IDCarona != "caro" {
		t.Fatalf("comportamento atual esperado era pegar a primeira aresta inserida (sem criterio de preco), veio %+v", resultado[0].Passos[0])
	}
}

func TestConstruirGrafo_VariasArestasMesmaOrigem(t *testing.T) {
	// Teste focado so em ConstruirGrafo (nao em BuscarItinerarios):
	// confirma que duas arestas com a MESMA cidade de origem viram 2
	// elementos na mesma chave do mapa, em vez de uma sobrescrever a
	// outra — o bug do "graph = append(...)" (sem reatribuir a chave)
	// do rascunho original, que perdia o mapa inteiro.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "B", 2, 10),
		aresta("c2", 0, "A", "B", 1, 20),
	})

	if len(grafo["A"]) != 2 {
		t.Fatalf("esperava 2 arestas saindo de A, veio %d: %+v", len(grafo["A"]), grafo["A"])
	}
}

// --- Casos de borda que geraram bugs no rascunho original -------------

func TestBuscarItinerarios_DestinoSemArestasSaindo(t *testing.T) {
	// B e fim de linha: ninguem sai de B para lugar nenhum, entao
	// grafo["B"] nunca chega a existir como chave do mapa (adjacencia e
	// indexada pela cidade de ORIGEM de cada aresta). Isso NAO pode
	// impedir B de ser um destino valido — foi exatamente o bug do
	// rascunho original, que checava grafo[destino] antes de buscar.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "B", 2, 15),
	})

	if _, existe := grafo["B"]; existe {
		t.Fatalf("premissa do teste quebrada: grafo[\"B\"] nao deveria existir como chave")
	}

	resultado := BuscarItinerarios(grafo, "A", "B")
	if len(resultado) != 1 {
		t.Fatalf("B deveria ser alcancavel mesmo sem nenhuma aresta saindo dele, veio %+v", resultado)
	}
}

func TestBuscarItinerarios_DestinoInalcancavel(t *testing.T) {
	// A e B existem no grafo (cada um com arestas saindo), mas nao ha
	// NENHUM caminho de A ate B — sao componentes desconectados do
	// grafo (ninguem liga o grupo A-X ao grupo Y-B).
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "X", 2, 10),
		aresta("c2", 0, "Y", "B", 2, 10),
	})

	resultado := BuscarItinerarios(grafo, "A", "B")
	if len(resultado) != 0 {
		t.Fatalf("esperava lista vazia (destino inalcancavel), veio %+v", resultado)
	}
}

func TestBuscarItinerarios_OrigemIgualDestino(t *testing.T) {
	// Caso trivial: se origem e destino sao a mesma cidade, nao ha
	// nenhum trecho a percorrer — a funcao devolve lista vazia em vez de
	// um itinerario com 0 passos.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("c1", 0, "A", "B", 2, 10),
	})

	resultado := BuscarItinerarios(grafo, "A", "A")
	if len(resultado) != 0 {
		t.Fatalf("origem igual a destino nao deveria gerar itinerario, veio %+v", resultado)
	}
}

func TestBuscarItinerarios_EvitaCiclos(t *testing.T) {
	// Grafo com um ciclo: A->B, B->A, B->C. Sem o controle de
	// "visitadas" dentro do BFS, isso faria a busca entrar num loop
	// A->B->A->B->... para sempre. Este teste garante que a busca
	// TERMINA e ainda encontra o caminho correto ate C.
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("ida", 0, "A", "B", 2, 10),
		aresta("volta", 0, "B", "A", 2, 10),
		aresta("saida", 0, "B", "C", 2, 10),
	})

	resultado := BuscarItinerarios(grafo, "A", "C")

	if len(resultado) != 1 || len(resultado[0].Passos) != 2 {
		t.Fatalf("esperava 1 itinerario com 2 trechos (A->B->C), veio %+v", resultado)
	}
}

// --- A garantia central do BFS: menos trechos, nao menor preco --------

func TestBuscarItinerarios_EscolheMenorNumeroDeTrechos(t *testing.T) {
	// Existe uma opcao DIRETA A->C (1 trecho, mais cara) e uma opcao
	// mais longa A->B->C (2 trechos, mais barata se somasse os precos).
	// BFS explora o grafo "em ondas" por numero de trechos — por isso
	// SEMPRE acha primeiro o caminho de MENOS trechos, nunca o mais
	// barato. Se no futuro voce quiser priorizar preco em vez de numero
	// de trocas, este e o teste que vai apontar a mudanca de
	// comportamento (ele vai passar a falhar, e esperado).
	grafo := ConstruirGrafo([]ArestaDisponivel{
		aresta("longa1", 0, "A", "B", 2, 5),
		aresta("longa2", 0, "B", "C", 2, 5),
		aresta("direta", 0, "A", "C", 2, 50), // so 1 trecho, mesmo sendo mais cara
	})

	resultado := BuscarTodosItinerarios(grafo, "A", "C")

	if len(resultado) != 1 || len(resultado[0].Passos) != 1 {
		t.Fatalf("esperava a rota direta de 1 trecho, veio %+v", resultado)
	}
	if resultado[0].Passos[0].IDCarona != "direta" {
		t.Fatalf("BFS nao escolheu o caminho de menos trechos: %+v", resultado[0].Passos[0])
	}
}

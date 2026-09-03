package dominio

// TODO — este arquivo deve conter a busca de itinerarios.
//
// Quando nenhuma carona cobre a origem-destino pedida diretamente, e
// preciso combinar trechos de caronas diferentes. Pense nisso como um
// grafo: cidades sao nos, trechos com assento disponivel na data pedida
// sao arestas. Buscar um itinerario de origem a destino e buscar um
// caminho nesse grafo (BFS/DFS ja resolve, dado o volume esperado).
//
// Esta busca deve operar sobre um SNAPSHOT de caronas passado como
// parametro (ex: []Carona) — ela nao acessa o estado global diretamente.
// Quem tira esse snapshot e chama esta funcao e o internal/estado, que
// tambem garante que o resultado ainda e valido no momento da leitura.

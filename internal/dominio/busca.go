package dominio

// TODO — este arquivo deve conter a busca de itinerarios.
//
// Quando nenhuma carona cobre a origem-destino pedida diretamente, e
// preciso combinar trechos de caronas diferentes. Pense nisso como um
// grafo: cidades sao nos, trechos com assento disponivel na data pedida
// sao arestas. Buscar um itinerario de origem a destino e buscar um
// caminho nesse grafo (BFS/DFS ja resolve, dado o volume esperado).
//
// Esta busca NAO deve travar nada — ela so le o estado atual e devolve
// possibilidades. A garantia de que o assento ainda existe e
// responsabilidade exclusiva da confirmacao (ver pacote concorrencia).

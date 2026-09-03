package main

import (
	"fmt"
	"log"

	"vaijunto/internal/clientenet"
	"vaijunto/internal/protocolo"
)

// Ponto de entrada do cliente motorista.
//
// Por enquanto so testa a conexao com um ping, para validar rede +
// protocolo + roteamento de ponta a ponta.
//
// TODO: substituir por um menu real (login, publicar carona, consultar
// caronas, cancelar carona) chamando internal/clientenet — ver README.md.
func main() {
	// Estabelece conexão 
	conexao, err := clientenet.Conectar("localhost:8080") 

	if err != nil {
		log.Fatal(err)
	}
	defer conexao.Fechar()

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "ping",
		IDRequisicao: "1",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("resposta do servidor: status=%s dados=%s\n", resp.Status, resp.Dados)
}

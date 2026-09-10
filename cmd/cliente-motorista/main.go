package main

// Ponto de entrada do cliente motorista.
//
// Por enquanto so testa a conexao com um ping, para validar rede +
// protocolo + roteamento de ponta a ponta.
//
// TODO: substituir por um menu real (login, publicar carona, consultar
// caronas, cancelar carona) chamando internal/clientenet — ver README.md.

import (
	"flag"
	"fmt"
	"log"

	"vaijunto/internal/clientenet"
)

func main() {
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
	flag.Parse()

	conexao, err := clientenet.Conectar(*endereco)
	if err != nil {
		log.Fatal(err)
	}

	defer conexao.Fechar()

	fmt.Printf("Motorista conectado ao servidor: %s\n", *endereco)

	menu(conexao)
}

func menu(conexao *clientenet.Conexao) {
	var choice int

	for {
		fmt.Println("\n--- MENU ---")
		fmt.Println("1. Publicar uma carona")
		fmt.Println("2. Consultar caronas disponíveis")
		fmt.Println("3. Cancelar uma carona")
		fmt.Println("4. Encerra conexão")
		fmt.Print("Enter your choice: ")

		fmt.Scan(&choice)

		switch choice {
		case 1:
			publicarCarona(conexao)
		case 2:
			consultarCaronas(conexao)
		case 3:
			cancelarCarona(conexao)
		default:
			fmt.Println("Invalid choice, please try again.")
		}
	}
}

// fmt.Printf("resposta do servidor: status=%s dados=%s\n", resp.Status, resp.Dados)

func publicarCarona(conexao *clientenet.Conexao) {
}

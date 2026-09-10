package main

import (
	"log"

	"vaijunto/internal/estado"
	"vaijunto/internal/rede"
)

// Ponto de entrada do servidor central do VaiJunto.
//
// O Repositorio e criado UMA UNICA VEZ aqui, no processo do servidor, e
// e o mesmo ponteiro compartilhado por todas as conexoes aceitas (via
// rede.Iniciar -> tratarConexao -> casosdeuso.Despachar). E isso que faz
// o estado (caronas, reservas) ser realmente compartilhado entre
// clientes diferentes, e nao um por conexao.
//
// TODO conforme o projeto crescer:
//   - ler porta/config de flag ou variavel de ambiente em vez do valor fixo abaixo;
//   - tratar encerramento gracioso (sinal SIGINT/SIGTERM).
func main() {
	repo := estado.NovoRepositorio()

	if err := rede.Iniciar(":8080", repo); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"log"

	"vaijunto/internal/rede"
)

// Ponto de entrada do servidor central do VaiJunto.
//
// TODO conforme o projeto crescer:
//   - ler porta/config de flag ou variavel de ambiente em vez do valor
//     fixo abaixo;
//   - inicializar internal/estado e passa-lo para internal/casosdeuso,
//     em vez do roteador so-com-ping que existe hoje;
//   - tratar encerramento gracioso (sinal SIGINT/SIGTERM).
func main() {
	if err := rede.Iniciar(":8080"); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"fmt"
	"log"

	"vaijunto/internal/clientenet"
	"vaijunto/internal/protocolo"
)

// Ponto de entrada do cliente passageiro.
//
// TODO:
//   1. Conectar ao servidor (ver internal/clientenet).
//   2. Autenticar o usuario (login).
//   3. Exibir um menu de acoes: buscar itinerarios entre origem/destino
//      numa data, confirmar reserva de um itinerario (um ou mais
//      trechos), consultar reservas, cancelar reserva.
//   4. Cada acao monta a mensagem do protocolo correspondente, envia via
//      internal/clientenet, e exibe a resposta ao usuario.
//
// Este arquivo deve conter APENAS a interface (menu/prompts) — nenhuma
// logica de rede ou de dominio deve morar aqui.

func main() {

	conexao, err := clientenet.Conectar("localhost:8080")

	if err != nil {
		// log.Fatal encerra o programa e imprime a mensagem de erro.
		log.Fatal(err)
	}
	// defer registra uma ação para ser executada ao final da função main.
	// Aqui, fechamos a conexão assim que o cliente terminar.
	defer conexao.Fechar()

	// O cliente solicita algo ao servidor enviando uma estrutura de protocolo.
	// 'Tipo' identifica a operação, e 'IDRequisicao' serve para rastrear a mensagem.
	// No fluxo cliente-servidor, o cliente envia a requisição e espera uma resposta.
	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "ping",
		IDRequisicao: "1",
	})
	if err != nil {
		// Qualquer erro na comunicação deve ser tratado imediatamente.
		log.Fatal(err)
	}

	fmt.Printf("resposta do servidor: status=%s dados=%s\n", resp.Status, resp.Dados)
}

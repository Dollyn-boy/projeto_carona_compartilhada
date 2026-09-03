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
	"vaijunto/internal/protocolo"
)

func main() {
	// O pacote "flag" (biblioteca padrao do Go) define argumentos de
	// linha de comando. flag.String(nome, valorPadrao, descricao) devolve
	// um *string: um PONTEIRO para uma variavel que so recebe o valor
	// real depois que flag.Parse() roda logo abaixo — por isso lemos com
	// *endereco (desreferenciando o ponteiro) mais adiante, e nao com
	// endereco direto.
	//
	// Isso substitui o "localhost:8080" que estava fixo no codigo. Por
	// que isso importa: "localhost" dentro de um container Docker (ou de
	// uma maquina diferente no laboratorio) aponta pro PROPRIO container,
	// nunca para o servidor rodando em outro lugar. Com a flag, o mesmo
	// binario funciona em qualquer cenario, so muda o valor passado:
	//   go run ./cmd/cliente-motorista                        (padrao: localhost:8080)
	//   go run ./cmd/cliente-motorista -servidor servidor:8080  (docker-compose, mesma maquina)
	//   go run ./cmd/cliente-motorista -servidor 192.168.0.10:8080  (maquina remota real)
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")

	// flag.Parse() le os argumentos de fato passados na linha de comando
	// (os.Args) e preenche a variavel apontada por "endereco". Precisa
	// ser chamado UMA vez, depois de todas as chamadas a flag.String/etc,
	// e antes de qualquer uso do valor.
	flag.Parse()

	// *endereco desreferencia o ponteiro devolvido acima, obtendo o
	// valor string de fato (o padrao "localhost:8080", ou o que tiver
	// sido passado via -servidor).
	conexao, err := clientenet.Conectar(*endereco)
	if err != nil {
		log.Fatal(err)
	}

	// defer registra uma ação para ser executada ao final da função main.
	// Aqui, fechamos a conexão assim que o cliente terminar.
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

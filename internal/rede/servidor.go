// Package rede é a camada "Rede (E/S)" do servidor: só fala TCP puro. Não
// sabe nada sobre caronas, reservas ou tipos de operação — apenas aceita
// conexões, lê e escreve mensagens já *formatadas* pelo pacote protocolo,
// e repassa cada mensagem recebida para o pacote casosdeuso decidir o
// que fazer com ela.
package rede

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"

	"vaijunto/internal/casosdeuso"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// Iniciar sobe o listener TCP no endereço informado (ex: ":8080") e
// entra no loop de aceitação de conexões. Bloqueia até o listener falhar
// ou ser fechado.
//
// CORRECAO: agora recebe *estado.Repositorio e repassa para cada
// conexão. Antes não existia NENHUMA forma de casosdeuso.Despachar
// chegar até o estado real — este é o parâmetro que fecha essa lacuna.
// O mesmo ponteiro de repo é compartilhado por TODAS as conexões
// aceitas (é o que faz uma carona publicada numa conexão aparecer pra
// consultas feitas por outra).
func Iniciar(endereco string, repo *estado.Repositorio) error {
	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		return fmt.Errorf("falha ao escutar em %s: %w", endereco, err)
	}
	defer listener.Close()

	log.Printf("servidor escutando em %s", endereco)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("falha ao aceitar conexao: %v", err)
			continue
		}

		go tratarConexao(conn, repo)
	}
}

// tratarConexao roda numa goroutine própria por cliente conectado — a
// queda de um cliente nunca afeta os demais. O bufio.Reader é criado uma
// única vez aqui e reaproveitado em todo o loop, pelo mesmo motivo
// explicado em internal/clientenet.
// rede/servidor.go

func tratarConexao(conn net.Conn, repo *estado.Repositorio) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// Cria uma sessão vazia (não autenticada) para esta conexão
	sessao := casosdeuso.NovaSessao()

	for {
		var req protocolo.Requisicao
		if err := protocolo.LerMensagem(reader, &req); err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("erro ao ler mensagem: %v", err)
			}
			return
		}

		// Passamos o PONTEIRO da sessão. Assim, o Despachar pode ler
		// para restringir acessos, ou alterar (ex: no login).
		resp := casosdeuso.Despachar(repo, sessao, req)

		if err := protocolo.EscreverMensagem(conn, resp); err != nil {
			log.Printf("erro ao enviar resposta: %v", err)
			return
		}
	}
}

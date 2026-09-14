// Package rede é a camada "Rede (E/S)" do servidor: só fala TCP puro. Não
// sabe nada sobre caronas, reservas ou tipos de operação — apenas aceita
// conexões, lê e escreve mensagens já *formatadas* pelo pacote protocolo,
// e repassa cada mensagem recebida para o pacote casosdeuso decidir o
// que fazer com ela.
package rede

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"vaijunto/internal/casosdeuso"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// timeoutInatividade encerra uma conexao que fica sem mandar NENHUMA
// mensagem por tempo demais — sem isso, um cliente que conecta e nunca
// fala nada (por bug, ma-fe, ou queda de rede sem FIN) prenderia a
// goroutine daquela conexao para sempre, vazando recursos aos poucos.
const timeoutInatividade = 5 * time.Minute

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
		// Renova o prazo a cada mensagem: o cliente tem ate
		// timeoutInatividade para mandar a PROXIMA mensagem, contando a
		// partir de agora — nao e um limite pra conexao inteira, so pra
		// ficar em silencio.
		conn.SetReadDeadline(time.Now().Add(timeoutInatividade))

		var req protocolo.Requisicao
		err := protocolo.LerMensagem(reader, &req)

		if err != nil {
			// EOF: o cliente fechou a conexao normalmente (ex: opcao
			// "encerrar" do menu). Nao e erro, nao loga.
			if errors.Is(err, io.EOF) {
				return
			}

			// Timeout: o cliente ficou quieto tempo demais. Registra e
			// encerra — a conexao provavelmente nao serve mais pra nada.
			var erroRede net.Error
			if errors.As(err, &erroRede) && erroRede.Timeout() {
				log.Printf("conexao encerrada por inatividade (%s sem mensagens)", timeoutInatividade)
				return
			}

			// Mensagem malformada (JSON invalido, campo com tipo errado):
			// a CONEXAO continua boa — so essa linha especifica nao fez
			// sentido. Avisa o cliente com uma resposta de erro e segue
			// lendo a proxima mensagem, em vez de derrubar tudo.
			var erroSintaxe *json.SyntaxError
			var erroTipo *json.UnmarshalTypeError
			if errors.As(err, &erroSintaxe) || errors.As(err, &erroTipo) {
				respErro := protocolo.Resposta{Status: "erro", Motivo: "mensagem malformada: " + err.Error()}
				if err := protocolo.EscreverMensagem(conn, respErro); err != nil {
					log.Printf("erro ao responder mensagem malformada: %v", err)
					return
				}
				continue
			}

			// Qualquer outro erro (conexao resetada, etc.) — nao da pra
			// recuperar, encerra.
			log.Printf("erro ao ler mensagem: %v", err)
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

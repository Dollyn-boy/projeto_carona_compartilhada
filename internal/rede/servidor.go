// Package rede implementa o servidor TCP e distribui as requisicoes para
// um conjunto fixo de workers.
package rede

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"vaijunto/internal/casosdeuso"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

const (
	timeoutInatividade = 5 * time.Minute
	timeoutEscrita     = 30 * time.Second
	filaPadrao         = 64
	workersPadrao      = 8
)

// workerPool mantem um numero fixo de goroutines processando funcoes.
// Uma fila limitada tambem aplica backpressure: quando enche, quem envia
// espera ate um worker ficar livre, em vez de criar goroutines sem limite.
type workerPool struct {
	fila chan func()
	wg   sync.WaitGroup
}

func novoWorkerPool(numeroWorkers, capacidadeFila int) *workerPool {
	if numeroWorkers < 1 {
		numeroWorkers = 1
	}
	if capacidadeFila < 1 {
		capacidadeFila = 1
	}
	p := &workerPool{fila: make(chan func(), capacidadeFila)}
	for i := 0; i < numeroWorkers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for tarefa := range p.fila {
				tarefa()
			}
		}()
	}
	return p
}

func (p *workerPool) enfileirar(tarefa func()) {
	p.fila <- tarefa
}

// fechar so deve ser chamado depois que todos os produtores pararem de enviar.
func (p *workerPool) fechar() {
	close(p.fila)
	p.wg.Wait()
}

// conexaoCliente centraliza escrita na conexao, evitando que respostas ou
// mensagens de erro misturem bytes caso dois caminhos precisem escrever.
type conexaoCliente struct {
	conn net.Conn
	mu   sync.Mutex
}

func (c *conexaoCliente) responder(resposta protocolo.Resposta) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(timeoutEscrita))
	return protocolo.EscreverMensagem(c.conn, resposta)
}

// Iniciar sobe o servidor usando uma quantidade fixa padrao de oito workers.
func Iniciar(endereco string, repo *estado.Repositorio) error {
	return IniciarComWorkers(endereco, repo, workersPadrao, filaPadrao)
}

// IniciarComWorkers e a versao configuravel, util para testes e experimentos
// de carga. A quantidade de workers limita quantas requisicoes sao executadas
// simultaneamente; a fila possui capacidade finita.
func IniciarComWorkers(endereco string, repo *estado.Repositorio, numeroWorkers, capacidadeFila int) error {
	if numeroWorkers < 1 {
		return fmt.Errorf("numero de workers precisa ser maior que zero")
	}
	if capacidadeFila < 1 {
		return fmt.Errorf("capacidade da fila precisa ser maior que zero")
	}
	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		return fmt.Errorf("falha ao escutar em %s: %w", endereco, err)
	}
	defer listener.Close()

	pool := novoWorkerPool(numeroWorkers, capacidadeFila)
	defer pool.fechar()

	log.Printf("servidor escutando em %s (workers=%d, fila=%d)", endereco, numeroWorkers, capacidadeFila)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("falha ao aceitar conexao: %v", err)
			continue
		}
		// Uma goroutine por conexao fica responsavel apenas por ler mensagens
		// e entrega-las a fila. A execucao das operacoes ocorre nos workers.
		go tratarConexao(conn, repo, pool)
	}
}

func tratarConexao(conn net.Conn, repo *estado.Repositorio, pool *workerPool) {
	defer conn.Close()
	cliente := &conexaoCliente{conn: conn}
	reader := bufio.NewReader(conn)
	sessao := casosdeuso.NovaSessao()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(timeoutInatividade))
		var req protocolo.Requisicao
		err := protocolo.LerMensagem(reader, &req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			var erroRede net.Error
			if errors.As(err, &erroRede) && erroRede.Timeout() {
				log.Printf("conexao encerrada por inatividade (%s sem mensagens)", timeoutInatividade)
				return
			}
			var erroSintaxe *json.SyntaxError
			var erroTipo *json.UnmarshalTypeError
			if errors.As(err, &erroSintaxe) || errors.As(err, &erroTipo) {
				respErro := protocolo.Resposta{Status: "erro", Motivo: "mensagem malformada: " + err.Error()}
				if err := cliente.responder(respErro); err != nil {
					log.Printf("erro ao responder mensagem malformada: %v", err)
					return
				}
				continue
			}
			log.Printf("erro ao ler mensagem: %v", err)
			return
		}

		// Cada conexao envia uma requisicao por vez: espera a resposta antes
		// de ler a proxima. Isso preserva a ordem das respostas e evita que a
		// sessao de login seja acessada simultaneamente por dois workers.
		concluida := make(chan struct{})
		pool.enfileirar(func() {
			defer close(concluida)
			resposta := casosdeuso.Despachar(repo, sessao, req)
			if err := cliente.responder(resposta); err != nil {
				log.Printf("erro ao enviar resposta: %v", err)
				_ = conn.Close()
			}
		})
		<-concluida
	}
}

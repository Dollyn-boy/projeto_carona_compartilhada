// Package clientenet é a camada de rede compartilhada pelos dois
// clientes (motorista e passageiro): conectar, enviar e receber
// mensagens do protocolo. É o equivalente, do lado do cliente, ao papel
// que internal/rede desempenha no servidor.
//
// Este pacote não contém nenhuma lógica de interface — só rede. A
// interface de terminal (menus, prompts) fica em cmd/cliente-motorista
// e cmd/cliente-passageiro, chamando este pacote.
package clientenet

import (
	"bufio"
	"fmt"
	"net"

	"vaijunto/internal/protocolo"
)

// Conexao empacota o socket TCP e o bufio.Reader associado a ele.
//
// O bufio.Reader precisa ser criado UMA VEZ por conexao e reaproveitado
// em toda leitura subsequente — criar um novo a cada chamada descartaria
// dados ja lidos do buffer interno (o mesmo bug do exercicio de eco).
type Conexao struct {
	conn   net.Conn
	reader *bufio.Reader
}

// Conectar abre uma conexao TCP com o servidor no endereco informado
// (ex: "localhost:8080") e prepara o reader que sera reutilizado
// durante toda a vida da conexao.
func Conectar(endereco string) (*Conexao, error) {
	conn, err := net.Dial("tcp", endereco)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar ao servidor: %w", err)
	}

	return &Conexao{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// Fechar encerra a conexao TCP.
func (c *Conexao) Fechar() error {
	return c.conn.Close()
}

// Enviar manda uma requisicao ao servidor e bloqueia ate receber a
// resposta correspondente. Como cada conexao processa uma requisicao
// por vez (sem pipelining), a proxima mensagem lida E a resposta desta
// requisicao — nao e preciso multiplexar por id_requisicao ainda.
func (c *Conexao) Enviar(req protocolo.Requisicao) (protocolo.Resposta, error) {
	var resp protocolo.Resposta

	if err := protocolo.EscreverMensagem(c.conn, req); err != nil {
		return resp, fmt.Errorf("falha ao enviar requisicao: %w", err)
	}

	if err := protocolo.LerMensagem(c.reader, &resp); err != nil {
		return resp, fmt.Errorf("falha ao ler resposta: %w", err)
	}

	return resp, nil
}

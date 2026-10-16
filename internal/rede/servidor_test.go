package rede

import (
	"bufio"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

func TestWorkerPoolLimitaProcessamentoSimultaneo(t *testing.T) {
	const quantidadeWorkers = 3
	const quantidadeTarefas = 15
	pool := novoWorkerPool(quantidadeWorkers, quantidadeTarefas)

	var ativos atomic.Int32
	var maximo atomic.Int32
	var wg sync.WaitGroup
	wg.Add(quantidadeTarefas)

	for i := 0; i < quantidadeTarefas; i++ {
		pool.enfileirar(func() {
			defer wg.Done()
			atual := ativos.Add(1)
			for {
				anterior := maximo.Load()
				if atual <= anterior || maximo.CompareAndSwap(anterior, atual) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			ativos.Add(-1)
		})
	}

	wg.Wait()
	pool.fechar()
	if obtido := maximo.Load(); obtido > quantidadeWorkers {
		t.Fatalf("processaram-se %d tarefas ao mesmo tempo, acima dos %d workers", obtido, quantidadeWorkers)
	}
	if obtido := maximo.Load(); obtido < 2 {
		t.Fatalf("teste nao conseguiu demonstrar processamento concorrente; maximo=%d", obtido)
	}
}

func TestWorkerPoolComValoresInvalidosNaoFicaSemWorkers(t *testing.T) {
	pool := novoWorkerPool(0, 0)
	concluida := make(chan struct{})
	pool.enfileirar(func() { close(concluida) })
	select {
	case <-concluida:
	case <-time.After(time.Second):
		t.Fatal("pool nao processou a tarefa")
	}
	pool.fechar()
}

func TestRequisicaoTCPPassaPeloWorkerPool(t *testing.T) {
	conexaoServidor, conexaoCliente := net.Pipe()
	// A conexaoServidor é usada pelo servidor, e a conexaoCliente é usada pelo teste para simular o cliente.
	pool := novoWorkerPool(1, 2)
	// O pool tem 1 worker e capacidade para 2 tarefas enfileiradas. Isso é suficiente para o teste, que envia apenas 1 requisicao.
	repo := estado.NovoRepositorio()

	// terminaServidor é um canal que será fechado quando a função tratarConexao terminar. Isso permite que o teste saiba quando o servidor terminou de processar a conexão.
	terminouServidor := make(chan struct{})
	go func() {
		tratarConexao(conexaoServidor, repo, pool)
		close(terminouServidor)
	}()
	defer func() {
		_ = conexaoCliente.Close()
		select {
		case <-terminouServidor:
		case <-time.After(time.Second):
			t.Error("a conexao nao terminou depois de o cliente fechar")
		}
		pool.fechar()
	}()

	requisicao := protocolo.Requisicao{Tipo: "ping", IDRequisicao: "teste-ping"}
	if err := protocolo.EscreverMensagem(conexaoCliente, requisicao); err != nil {
		t.Fatal(err)
	}
	var resposta protocolo.Resposta
	if err := protocolo.LerMensagem(bufio.NewReader(conexaoCliente), &resposta); err != nil {
		t.Fatal(err)
	}
	if resposta.Status != "ok" || resposta.IDRequisicao != requisicao.IDRequisicao {
		t.Fatalf("resposta inesperada: %+v", resposta)
	}

	print("resposta recebida: ", string(resposta.Dados), "\n")
}

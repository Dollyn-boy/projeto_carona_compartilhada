package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"vaijunto/internal/api"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/estado"
	"vaijunto/internal/rede"
)

func main() {
	// Identidade desta empresa.
	empresaID := strings.TrimSpace(os.Getenv("VAIJUNTO_EMPRESA_ID"))
	if empresaID == "" {
		empresaID = "vaijunto-local"
	}

	// Endereco do servidor TCP para os clientes.
	enderecoTCP := strings.TrimSpace(os.Getenv("VAIJUNTO_ENDERECO"))
	if enderecoTCP == "" {
		enderecoTCP = ":8080"
	}

	// Endereco da API REST para comunicacao entre empresas.
	enderecoREST := strings.TrimSpace(os.Getenv("VAIJUNTO_REST_ENDERECO"))
	if enderecoREST == "" {
		enderecoREST = ":8085"
	}

	// Arquivo JSON exclusivo desta empresa.
	arquivo := strings.TrimSpace(os.Getenv("VAIJUNTO_ARQUIVO_DADOS"))
	if arquivo == "" {
		arquivo = filepath.Join("dados", "dados.json")
	}

	// Carrega o repositorio da empresa.
	repo, err := estado.NovoRepositorioDeEmpresaComArquivo(
		empresaID,
		arquivo,
	)
	if err != nil {
		log.Fatalf("nao foi possivel abrir os dados: %v", err)
	}

	log.Printf(
		"empresa=%s; dados carregados de %s",
		empresaID,
		arquivo,
	)

	// Le as empresas que esta instancia deve consultar.
	peers, err := carregarPeers(empresaID, os.Getenv("VAIJUNTO_PEERS"))
	if err != nil {
		log.Fatalf("configuracao de empresas parceiras invalida: %v", err)
	}

	// Servico responsavel pela busca distribuida.
	busca := consorcio.NovoBuscaDistribuida(repo, peers)

	// Recupera coordenacoes pendentes do arquivo JSON e continua tentando
	// entregar decisoes duraveis enquanto os peers estiverem indisponiveis.
	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()
	var recuperacaoWG sync.WaitGroup
	recuperacaoWG.Add(1)
	go func() {
		defer recuperacaoWG.Done()
		executarRecuperacao(ctx, busca.Coordenador)
	}()

	// Inicia as duas interfaces. A rotina de recuperacao e independente dos
	// listeners: falhas temporarias nos peers nao impedem a API/TCP de subir.
	erros := make(chan error, 2)
	go func() {
		log.Printf("empresa=%s; API REST escutando em %s", empresaID, enderecoREST)
		erros <- api.IniciarComPeersContext(ctx, enderecoREST, empresaID, repo, peers)
	}()
	go func() {
		log.Printf("empresa=%s; servidor TCP escutando em %s", empresaID, enderecoTCP)
		erros <- rede.IniciarComBuscaContext(ctx, enderecoTCP, repo, busca)
	}()

	var erroServidor error
	recebidos := 0
	select {
	case <-ctx.Done():
		log.Printf("empresa=%s; encerramento solicitado; aguardando os listeners", empresaID)
	case err := <-erros:
		recebidos++
		if err != nil {
			erroServidor = err
		} else {
			erroServidor = fmt.Errorf("um listener encerrou inesperadamente sem erro")
		}
		cancelar()
	}

	for recebidos < 2 {
		err := <-erros
		recebidos++
		if err != nil {
			if erroServidor == nil {
				erroServidor = err
			} else {
				log.Printf("erro adicional durante encerramento: %v", err)
			}
		}
	}
	recuperacaoWG.Wait()
	if erroServidor != nil {
		log.Fatalf("servidor encerrado por erro: %v", erroServidor)
	}
	log.Printf("empresa=%s; listeners e recuperacao encerrados", empresaID)
}

// executarRecuperacao tenta imediatamente recuperar o log local e repete a
// operacao periodicamente. A decisao duravel nunca e alterada durante retry.
func executarRecuperacao(ctx context.Context, coordenador *consorcio.Coordenador) {
	const intervalo = 5 * time.Second
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()

	tentar := func() {
		ctxTentativa, cancelar := context.WithTimeout(ctx, 12*time.Second)
		defer cancelar()
		if err := coordenador.RecuperarPendentes(ctxTentativa); err != nil {
			log.Printf("recuperacao de transacoes pendentes: %v", err)
		}
	}

	tentar()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tentar()
		}
	}
}

// carregarPeers interpreta:
//
// empresa-b=http://servidor-b:8085,empresa-c=http://servidor-c:8085
//
// Cada empresa deve informar somente as outras empresas, nunca a si propria.
func carregarPeers(empresaLocal, configuracao string) ([]consorcio.Peer, error) {
	configuracao = strings.TrimSpace(configuracao)
	if configuracao == "" {
		return nil, nil
	}

	itens := strings.Split(configuracao, ",")
	peers := make([]consorcio.Peer, 0, len(itens))
	ids := make(map[string]bool)

	for _, item := range itens {
		partes := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(partes) != 2 {
			return nil, fmt.Errorf(
				"entrada %q deve seguir o formato empresa=url",
				item,
			)
		}

		id := strings.TrimSpace(partes[0])
		endereco := strings.TrimRight(strings.TrimSpace(partes[1]), "/")

		if id == "" || endereco == "" {
			return nil, fmt.Errorf("empresa e URL nao podem ser vazias: %q", item)
		}

		if id == empresaLocal {
			return nil, fmt.Errorf(
				"a empresa %q nao deve aparecer na lista de parceiras",
				id,
			)
		}

		if ids[id] {
			return nil, fmt.Errorf("empresa parceira duplicada: %q", id)
		}

		u, err := url.ParseRequestURI(endereco)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("URL invalida para %q: %q", id, endereco)
		}

		ids[id] = true
		peers = append(peers, consorcio.Peer{
			EmpresaID: id,
			URL:       endereco,
		})
	}

	return peers, nil
}

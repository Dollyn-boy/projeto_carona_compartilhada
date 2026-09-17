
//  1. Cadastra um motorista de teste e publica UMA carona com
//     capacidade conhecida (N assentos).
//  2. Dispara M passageiros CONCORRENTES (goroutines, cada um com sua
//     PROPRIA conexao TCP), todos tentando confirmar reserva no MESMO
//     trecho dessa carona ao mesmo tempo.
//  3. Verifica a invariante central do projeto: o numero de
//     confirmacoes bem-sucedidas tem que ser EXATAMENTE igual a
//     capacidade — nem a mais (venda dupla) nem a menos (assento
//     perdido por erro de concorrencia).
//  4. Mede o tempo de resposta (mediana e p99) sob essa disputa.
//
//
//	go run ./test/carga -servidor localhost:8080 -clientes 30 -capacidade 5
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"vaijunto/internal/clientenet"
	"vaijunto/internal/protocolo"
)

func main() {
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
	nClientes := flag.Int("clientes", 30, "numero de passageiros concorrentes disputando o mesmo trecho")
	capacidade := flag.Int("capacidade", 5, "capacidade de assentos da carona de teste")
	flag.Parse()

	idCarona := publicarCaronaDeTeste(*endereco, *capacidade)
	fmt.Printf("Carona de teste publicada: id=%d capacidade=%d\n", idCarona, *capacidade)
	fmt.Printf("Disparando %d passageiros concorrentes contra o mesmo trecho...\n\n", *nClientes)

	resultados := make([]resultado, *nClientes)
	var wg sync.WaitGroup

	inicio := time.Now()
	for i := 0; i < *nClientes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resultados[idx] = tentarReservar(*endereco, idx, idCarona)
		}(i)
	}
	wg.Wait()
	duracaoTotal := time.Since(inicio)

	relatorio(resultados, *capacidade, duracaoTotal)
}

// resultado captura o desfecho de UMA tentativa de reserva, para
// agregacao no relatorio final.
type resultado struct {
	sucesso bool
	duracao time.Duration
	motivo  string
}

// publicarCaronaDeTeste cadastra um motorista dedicado e publica uma
// carona com a capacidade pedida — e o "alvo" que todos os passageiros
// vao disputar depois.
func publicarCaronaDeTeste(endereco string, capacidade int) int {
	conexao, err := clientenet.Conectar(endereco)
	if err != nil {
		log.Fatal("falha ao conectar para publicar carona de teste: ", err)
	}
	defer conexao.Fechar()

	usuario := fmt.Sprintf("motorista-carga-%d", time.Now().UnixNano())
	cadastroDados, _ := json.Marshal(protocolo.CadastroDados{Usuario: usuario, Senha: "senha123", Role: "m"})
	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "cadastro", IDRequisicao: "setup-1", Dados: cadastroDados})
	if err != nil || resp.Status != "ok" {
		log.Fatalf("falha ao cadastrar motorista de teste: err=%v resp=%+v", err, resp)
	}

	rota := []string{"CidadeA-Carga", "CidadeB-Carga"}
	data := time.Now().Add(24 * time.Hour).Format("2006-01-02")
	publicarDados, _ := json.Marshal(protocolo.PublicarCaronaDados{
		Rota: rota, Capacidade: capacidade, Preco: 10, Data: data,
	})
	resp, err = conexao.Enviar(protocolo.Requisicao{Tipo: "publicar_carona", IDRequisicao: "setup-2", Dados: publicarDados})
	if err != nil || resp.Status != "ok" {
		log.Fatalf("falha ao publicar carona de teste: err=%v resp=%+v", err, resp)
	}

	var carona protocolo.CaronaResposta
	if err := json.Unmarshal(resp.Dados, &carona); err != nil {
		log.Fatal("resposta inesperada ao publicar carona de teste: ", err)
	}
	return carona.ID
}

// tentarReservar simula UM passageiro: abre sua PROPRIA conexao TCP
// (importante — nao reaproveita a conexao de ninguem, cada passageiro
// e independente, exatamente como aconteceria com clientes reais em
// maquinas diferentes), cadastra-se, e tenta confirmar reserva no
// trecho unico da carona de teste (sempre indice 0, ja que a rota so
// tem 2 cidades).
func tentarReservar(endereco string, idx int, idCarona int) resultado {
	inicio := time.Now()

	conexao, err := clientenet.Conectar(endereco)
	if err != nil {
		return resultado{sucesso: false, duracao: time.Since(inicio), motivo: "erro de conexao: " + err.Error()}
	}
	defer conexao.Fechar()

	usuario := fmt.Sprintf("passageiro-carga-%d-%d", idx, time.Now().UnixNano())
	cadastroDados, _ := json.Marshal(protocolo.CadastroDados{Usuario: usuario, Senha: "senha123", Role: "p"})
	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "cadastro", IDRequisicao: "1", Dados: cadastroDados})
	if err != nil {
		return resultado{sucesso: false, duracao: time.Since(inicio), motivo: "erro de rede no cadastro: " + err.Error()}
	}
	if resp.Status != "ok" {
		return resultado{sucesso: false, duracao: time.Since(inicio), motivo: "falha no cadastro: " + resp.Motivo}
	}

	trecho := protocolo.TrechoDados{
		IDCarona: fmt.Sprintf("%d", idCarona),
		Indice:   0,
		Origem:   "CidadeA-Carga",
		Destino:  "CidadeB-Carga",
	}
	confirmarDados, _ := json.Marshal(protocolo.ConfirmarReservaDados{Trechos: []protocolo.TrechoDados{trecho}})

	resp, err = conexao.Enviar(protocolo.Requisicao{Tipo: "confirmar_reserva", IDRequisicao: "2", Dados: confirmarDados})
	duracao := time.Since(inicio)
	if err != nil {
		return resultado{sucesso: false, duracao: duracao, motivo: "erro de rede na confirmacao: " + err.Error()}
	}
	if resp.Status != "ok" {
		return resultado{sucesso: false, duracao: duracao, motivo: resp.Motivo}
	}
	return resultado{sucesso: true, duracao: duracao}
}

// relatorio imprime o resumo final e confere a invariante central do
// projeto: sucessos == capacidade, nem mais nem menos.
func relatorio(resultados []resultado, capacidade int, duracaoTotal time.Duration) {
	sucessos := 0
	duracoes := make([]time.Duration, 0, len(resultados))
	motivosFalha := map[string]int{}

	for _, r := range resultados {
		duracoes = append(duracoes, r.duracao)
		if r.sucesso {
			sucessos++
		} else {
			motivosFalha[r.motivo]++
		}
	}

	fmt.Println("=== RESULTADO DO TESTE DE CARGA ===")
	fmt.Printf("Clientes concorrentes:     %d\n", len(resultados))
	fmt.Printf("Capacidade da carona:      %d\n", capacidade)
	fmt.Printf("Confirmacoes com sucesso:  %d\n", sucessos)
	fmt.Printf("Tempo total do teste:      %s\n", duracaoTotal)
	fmt.Println()

	// A INVARIANTE que este teste existe pra provar.
	if sucessos == capacidade {
		fmt.Printf("[OK] %d confirmacoes bateram exatamente com a capacidade — nenhum assento vendido a mais, nenhum perdido.\n", sucessos)
	} else if sucessos > capacidade {
		fmt.Printf("[FALHA CRITICA] %d confirmacoes > capacidade (%d) — assento vendido em dobro!\n", sucessos, capacidade)
	} else {
		fmt.Printf("[FALHA] %d confirmacoes < capacidade (%d) — algum passageiro deveria ter conseguido reservar e nao conseguiu.\n", sucessos, capacidade)
	}

	if len(motivosFalha) > 0 {
		fmt.Println("\nMotivos de falha (esperado: a maioria \"sem assento disponivel\", ja que a disputa é o ponto do teste):")
		for motivo, contagem := range motivosFalha {
			fmt.Printf("  [%dx] %s\n", contagem, motivo)
		}
	}

	sort.Slice(duracoes, func(i, j int) bool { return duracoes[i] < duracoes[j] })
	if len(duracoes) > 0 {
		p50 := duracoes[len(duracoes)*50/100]
		p99 := duracoes[min(len(duracoes)*99/100, len(duracoes)-1)]
		fmt.Printf("\nTempo de resposta sob disputa — mediana (p50): %s | p99: %s | maximo: %s\n",
			p50, p99, duracoes[len(duracoes)-1])
	}
}

package main

// Ponto de entrada do cliente motorista. Menu de terminal completo:
// publicar carona, consultar caronas, cancelar carona.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"vaijunto/internal/clientenet"
	"vaijunto/internal/protocolo"
)

// proximoID e um contador simples para IDRequisicao. Como este cliente
// e um processo de terminal (nao concorrente entre si), nao precisa de
// mutex — so uma goroutine (a main) chama isto.
var proximoID = 0

func main() {
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
	flag.Parse()

	conexao, err := clientenet.Conectar(*endereco)
	if err != nil {
		log.Fatal(err)
	}
	defer conexao.Fechar()

	fmt.Printf("Motorista conectado ao servidor: %s\n", *endereco)

	leitor := bufio.NewReader(os.Stdin)
	motorista := lerLinha(leitor, "Seu nome (motorista): ")

	menu(conexao, leitor, motorista)
}

func menu(conexao *clientenet.Conexao, leitor *bufio.Reader, motorista string) {
	for {
		fmt.Println("\n--- MENU MOTORISTA ---")
		fmt.Println("1. Publicar uma carona")
		fmt.Println("2. Consultar caronas disponíveis")
		fmt.Println("3. Cancelar uma carona")
		fmt.Println("4. Encerrar conexão")

		escolha := lerInt(leitor, "Escolha: ")

		switch escolha {
		case 1:
			publicarCarona(conexao, leitor, motorista)
		case 2:
			consultarCaronas(conexao, motorista)
		case 3:
			cancelarCarona(conexao, leitor)
		case 4:
			fmt.Println("Até mais!")
			return
		default:
			fmt.Println("Opção inválida, tente de novo.")
		}
	}
}

func publicarCarona(conexao *clientenet.Conexao, leitor *bufio.Reader, motorista string) {
	rotaTexto := lerLinha(leitor, "Rota (cidades separadas por vírgula, ex: Salvador,Feira de Santana): ")
	rota := strings.Split(rotaTexto, ",")
	for i := range rota {
		rota[i] = strings.TrimSpace(rota[i])
	}

	capacidade := lerInt(leitor, "Capacidade de assentos: ")
	preco := lerInt(leitor, "Preço: ")
	data := lerLinha(leitor, "Data (AAAA-MM-DD): ")

	dados, _ := json.Marshal(protocolo.PublicarCaronaDados{
		Motorista:  motorista,
		Rota:       rota,
		Capacidade: capacidade,
		Preco:      preco,
		Data:       data,
	})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "publicar_carona",
		IDRequisicao: novoIDRequisicao(),
		Dados:        dados,
	})
	if err != nil {
		fmt.Println("erro de rede:", err)
		return
	}
	if resp.Status != "ok" {
		fmt.Println("Erro:", resp.Motivo)
		return
	}

	var carona protocolo.CaronaResposta
	if err := json.Unmarshal(resp.Dados, &carona); err != nil {
		fmt.Println("resposta inesperada do servidor:", err)
		return
	}
	fmt.Printf("Carona publicada! ID=%d rota=%v capacidade=%d preco=%d data=%s\n",
		carona.ID, carona.Rota, carona.Capacidade, carona.Preco, carona.Data)
}

func consultarCaronas(conexao *clientenet.Conexao, motorista string) {
	dados, _ := json.Marshal(protocolo.ConsultarCaronasDados{Motorista: motorista})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "consultar_caronas",
		IDRequisicao: novoIDRequisicao(),
		Dados:        dados,
	})
	if err != nil {
		fmt.Println("erro de rede:", err)
		return
	}
	if resp.Status != "ok" {
		fmt.Println("Erro:", resp.Motivo)
		return
	}

	var resultado protocolo.ConsultarCaronasResposta
	if err := json.Unmarshal(resp.Dados, &resultado); err != nil {
		fmt.Println("resposta inesperada do servidor:", err)
		return
	}

	if len(resultado.Caronas) == 0 {
		fmt.Println("Nenhuma carona publicada ainda.")
		return
	}
	for _, c := range resultado.Caronas {
		fmt.Printf("  [ID %d] %v — %d assentos, R$%d, %s\n", c.ID, c.Rota, c.Capacidade, c.Preco, c.Data)
	}
}

func cancelarCarona(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	id := lerInt(leitor, "ID da carona a cancelar: ")

	dados, _ := json.Marshal(protocolo.CancelarCaronaDados{IDCarona: id})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "cancelar_carona",
		IDRequisicao: novoIDRequisicao(),
		Dados:        dados,
	})
	if err != nil {
		fmt.Println("erro de rede:", err)
		return
	}
	if resp.Status != "ok" {
		fmt.Println("Erro:", resp.Motivo)
		return
	}
	fmt.Println("Carona cancelada.")
}

// --- helpers de entrada/saida (duplicados em cliente-passageiro; sao
// poucas linhas e cada cmd/ e um binario independente — se crescer,
// vale extrair para um pacote interno compartilhado) ---

// lerLinha le uma linha inteira do terminal. Usar SEMPRE o mesmo
// bufio.Reader (criado uma vez em main) para todo input — misturar
// fmt.Scan (que nao consome o '\n' final) com ReadString('\n') e uma
// fonte classica de bug: o '\n' que fmt.Scan deixou pra tras vira uma
// linha vazia na proxima leitura.
func lerLinha(leitor *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	linha, _ := leitor.ReadString('\n')
	return strings.TrimSpace(linha)
}

// lerInt le uma linha e tenta converter para int, pedindo de novo se
// falhar — evita usar fmt.Scan(&int) pelo motivo explicado acima.
func lerInt(leitor *bufio.Reader, prompt string) int {
	for {
		linha := lerLinha(leitor, prompt)
		valor, err := strconv.Atoi(linha)
		if err == nil {
			return valor
		}
		fmt.Println("valor invalido, digite um numero.")
	}
}

func novoIDRequisicao() string {
	proximoID++
	return strconv.Itoa(proximoID)
}

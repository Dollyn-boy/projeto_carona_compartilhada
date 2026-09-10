package main

// Ponto de entrada do cliente passageiro. Menu de terminal completo:
// buscar itinerarios, confirmar reserva, consultar reservas, cancelar
// reserva.

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

var proximoID = 0

// ultimaBusca guarda os itinerarios devolvidos pela ultima busca, para
// que "confirmar reserva" possa referenciar um deles por indice sem o
// passageiro precisar redigitar cidade por cidade e carona por carona.
// Isso e so estado de SESSAO do terminal (memoria do processo cliente),
// nao tem nada a ver com o estado do servidor.
var ultimaBusca []protocolo.ItinerarioDados

func main() {
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
	flag.Parse()

	conexao, err := clientenet.Conectar(*endereco)
	if err != nil {
		log.Fatal(err)
	}
	defer conexao.Fechar()

	fmt.Printf("Passageiro conectado ao servidor: %s\n", *endereco)

	leitor := bufio.NewReader(os.Stdin)
	passageiro := lerLinha(leitor, "Seu nome (passageiro): ")

	menu(conexao, leitor, passageiro)
}

func menu(conexao *clientenet.Conexao, leitor *bufio.Reader, passageiro string) {
	for {
		fmt.Println("\n--- MENU PASSAGEIRO ---")
		fmt.Println("1. Buscar itinerários")
		fmt.Println("2. Confirmar reserva (de uma busca recente)")
		fmt.Println("3. Consultar minhas reservas")
		fmt.Println("4. Cancelar reserva")
		fmt.Println("5. Encerrar conexão")

		escolha := lerInt(leitor, "Escolha: ")

		switch escolha {
		case 1:
			buscarItinerarios(conexao, leitor)
		case 2:
			confirmarReserva(conexao, leitor, passageiro)
		case 3:
			consultarReservas(conexao, passageiro)
		case 4:
			cancelarReserva(conexao, leitor)
		case 5:
			fmt.Println("Até mais!")
			return
		default:
			fmt.Println("Opção inválida, tente de novo.")
		}
	}
}

func buscarItinerarios(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	origem := lerLinha(leitor, "Origem: ")
	destino := lerLinha(leitor, "Destino: ")
	data := lerLinha(leitor, "Data (AAAA-MM-DD): ")

	dados, _ := json.Marshal(protocolo.BuscarItinerariosDados{
		Origem:  origem,
		Destino: destino,
		Data:    data,
	})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "buscar_itinerarios",
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

	var resultado protocolo.BuscarItinerariosResposta
	if err := json.Unmarshal(resp.Dados, &resultado); err != nil {
		fmt.Println("resposta inesperada do servidor:", err)
		return
	}

	ultimaBusca = resultado.Itinerarios // guardado para a opcao "confirmar reserva"

	if len(resultado.Itinerarios) == 0 {
		fmt.Println("Nenhum itinerário encontrado.")
		return
	}

	fmt.Println("Itinerários encontrados:")
	for i, it := range resultado.Itinerarios {
		var partes []string
		for _, t := range it.Trechos {
			partes = append(partes, fmt.Sprintf("%s->%s (carona %s)", t.Origem, t.Destino, t.IDCarona))
		}
		fmt.Printf("  [%d] %s\n", i, strings.Join(partes, "  |  "))
	}
}

func confirmarReserva(conexao *clientenet.Conexao, leitor *bufio.Reader, passageiro string) {
	if len(ultimaBusca) == 0 {
		fmt.Println("Busque um itinerário primeiro (opção 1).")
		return
	}

	indice := lerInt(leitor, fmt.Sprintf("Qual itinerário confirmar? (0 a %d): ", len(ultimaBusca)-1))
	if indice < 0 || indice >= len(ultimaBusca) {
		fmt.Println("Índice inválido.")
		return
	}

	dados, _ := json.Marshal(protocolo.ConfirmarReservaDados{
		Passageiro: passageiro,
		Trechos:    ultimaBusca[indice].Trechos,
	})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "confirmar_reserva",
		IDRequisicao: novoIDRequisicao(),
		Dados:        dados,
	})
	if err != nil {
		fmt.Println("erro de rede:", err)
		return
	}
	if resp.Status != "ok" {
		// Esperado acontecer sob concorrencia: outro passageiro pode ter
		// confirmado o mesmo trecho primeiro entre a busca e agora.
		fmt.Println("Não foi possível confirmar:", resp.Motivo)
		return
	}

	var reserva protocolo.ReservaResposta
	if err := json.Unmarshal(resp.Dados, &reserva); err != nil {
		fmt.Println("resposta inesperada do servidor:", err)
		return
	}
	fmt.Printf("Reserva confirmada! ID=%d\n", reserva.ID)
}

func consultarReservas(conexao *clientenet.Conexao, passageiro string) {
	dados, _ := json.Marshal(protocolo.ConsultarReservasDados{Passageiro: passageiro})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "consultar_reservas",
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

	var resultado protocolo.ConsultarReservasResposta
	if err := json.Unmarshal(resp.Dados, &resultado); err != nil {
		fmt.Println("resposta inesperada do servidor:", err)
		return
	}

	if len(resultado.Reservas) == 0 {
		fmt.Println("Nenhuma reserva encontrada.")
		return
	}
	for _, r := range resultado.Reservas {
		var partes []string
		for _, t := range r.Trechos {
			partes = append(partes, fmt.Sprintf("%s->%s (carona %s)", t.Origem, t.Destino, t.IDCarona))
		}
		fmt.Printf("  [ID %d] %s\n", r.ID, strings.Join(partes, "  |  "))
	}
}

func cancelarReserva(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	id := lerInt(leitor, "ID da reserva a cancelar: ")

	dados, _ := json.Marshal(protocolo.CancelarReservaDados{IDReserva: id})

	resp, err := conexao.Enviar(protocolo.Requisicao{
		Tipo:         "cancelar_reserva",
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
	fmt.Println("Reserva cancelada.")
}

// --- helpers de entrada/saida (mesmos de cliente-motorista) ---

func lerLinha(leitor *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	linha, _ := leitor.ReadString('\n')
	return strings.TrimSpace(linha)
}

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

package main

// Cliente unico do VaiJunto: pede cadastro ou login (com o papel —
// motorista ou passageiro — escolhido no cadastro e devolvido pelo
// login), e a partir dai mostra o menu certo. Substitui os antigos
// cmd/cliente-motorista e cmd/cliente-passageiro, que nunca faziam
// login de verdade e por isso ficaram incompativeis quando o servidor
// passou a exigir autenticacao em toda operacao privada.

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

// ultimaBusca guarda os itinerarios da ultima busca do passageiro, para
// "confirmar reserva" poder referenciar um deles por indice. E so
// estado de sessao do terminal, nao tem nada a ver com o servidor.
var ultimaBusca []protocolo.ItinerarioDados

func main() {
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
	flag.Parse()

	conexao, err := clientenet.Conectar(*endereco)
	if err != nil {
		log.Fatal(err)
	}
	defer conexao.Fechar()

	fmt.Printf("Conectado ao servidor: %s\n", *endereco)

	leitor := bufio.NewReader(os.Stdin)

	for {
		opcao := lerLinha(leitor, "Você deseja se cadastrar ou autenticar? (c/a): ")
		if opcao == "c" {
			cadastrarCliente(conexao, leitor)
			return
		} else if opcao == "a" {
			loginCliente(conexao, leitor)
			return
		}
		fmt.Println("Opção inválida. Digite 'c' para cadastrar ou 'a' para autenticar.")
	}
}

func cadastrarCliente(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	for {
		role := lerLinha(leitor, "Você é motorista ou passageiro? (m/p): ")
		if role != "m" && role != "p" {
			fmt.Println("Papel inválido. Digite 'm' ou 'p'.")
			continue
		}

		usuario := lerLinha(leitor, "Nome de usuário: ")
		senha := lerLinha(leitor, "Senha: ")

		dados, _ := json.Marshal(protocolo.CadastroDados{Usuario: usuario, Role: role, Senha: senha})

		resp, err := conexao.Enviar(protocolo.Requisicao{
			Tipo:         "cadastro",
			IDRequisicao: novoIDRequisicao(),
			Dados:        dados,
		})
		if err != nil {
			log.Fatal("erro de rede ao cadastrar: ", err)
		}

		if resp.Status == "ok" {
			fmt.Println("\nCadastro realizado com sucesso!")
			// Cadastrar nao loga automaticamente (o servidor so cria o
			// usuario) — o proprio usuario ja sabe seu role, escolhido
			// agora mesmo, entao pulamos direto pro menu certo.
			menu(conexao, leitor, role)
			return
		}
		fmt.Printf("\nFalha no cadastro: %s\nTente novamente.\n\n", resp.Motivo)
	}
}

func loginCliente(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	for {
		usuario := lerLinha(leitor, "Nome de usuário: ")
		senha := lerLinha(leitor, "Senha: ")

		dados, _ := json.Marshal(protocolo.LoginDados{Usuario: usuario, Senha: senha})

		resp, err := conexao.Enviar(protocolo.Requisicao{
			Tipo:         "login",
			IDRequisicao: novoIDRequisicao(),
			Dados:        dados,
		})
		if err != nil {
			log.Fatal("erro de rede ao logar: ", err)
		}

		if resp.Status != "ok" {
			fmt.Printf("\nFalha no login: %s\nTente novamente.\n\n", resp.Motivo)
			continue
		}

		var loginResp protocolo.LoginResposta
		if err := json.Unmarshal(resp.Dados, &loginResp); err != nil {
			log.Fatal("resposta de login inesperada: ", err)
		}
		fmt.Println("\n" + loginResp.Mensagem)
		menu(conexao, leitor, loginResp.Role)
		return
	}
}

// menu direciona para o menu certo conforme o Role devolvido pelo
// servidor (nunca escolhido livremente pelo cliente depois do login).
func menu(conexao *clientenet.Conexao, leitor *bufio.Reader, role string) {
	switch role {
	case "m":
		menuMotorista(conexao, leitor)
	case "p":
		menuPassageiro(conexao, leitor)
	default:
		fmt.Println("Role desconhecido:", role)
	}
}

// ============================================================================
// Menu do motorista
// ============================================================================

func menuMotorista(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	for {
		fmt.Println("\n--- MENU MOTORISTA ---")
		fmt.Println("1. Publicar uma carona")
		fmt.Println("2. Consultar minhas caronas")
		fmt.Println("3. Cancelar uma carona")
		fmt.Println("4. Encerrar conexão")

		switch lerInt(leitor, "Escolha: ") {
		case 1:
			publicarCarona(conexao, leitor)
		case 2:
			consultarCaronas(conexao)
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

func publicarCarona(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	rotaTexto := lerLinha(leitor, "Rota (cidades separadas por vírgula, ex: Salvador,Feira de Santana): ")
	rota := strings.Split(rotaTexto, ",")
	for i := range rota {
		rota[i] = strings.TrimSpace(rota[i])
	}

	capacidade := lerInt(leitor, "Capacidade de assentos: ")
	preco := lerInt(leitor, "Preço: ")
	data := lerLinha(leitor, "Data (AAAA-MM-DD): ")

	dados, _ := json.Marshal(protocolo.PublicarCaronaDados{Rota: rota, Capacidade: capacidade, Preco: preco, Data: data})

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "publicar_carona", IDRequisicao: novoIDRequisicao(), Dados: dados})
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

func consultarCaronas(conexao *clientenet.Conexao) {
	dados, _ := json.Marshal(protocolo.ConsultarCaronasDados{})

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "consultar_caronas", IDRequisicao: novoIDRequisicao(), Dados: dados})
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
		for i, passageiros := range c.PassageirosPorTrecho {
			origem, destino := c.Rota[i], c.Rota[i+1]
			if len(passageiros) == 0 {
				fmt.Printf("      trecho %s->%s: vazio\n", origem, destino)
				continue
			}
			fmt.Printf("      trecho %s->%s: %s\n", origem, destino, strings.Join(passageiros, ", "))
		}
	}
}

func cancelarCarona(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	id := lerInt(leitor, "ID da carona a cancelar: ")
	dados, _ := json.Marshal(protocolo.CancelarCaronaDados{IDCarona: id})

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "cancelar_carona", IDRequisicao: novoIDRequisicao(), Dados: dados})
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

// ============================================================================
// Menu do passageiro
// ============================================================================

func menuPassageiro(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	for {
		fmt.Println("\n--- MENU PASSAGEIRO ---")
		fmt.Println("1. Buscar itinerários")
		fmt.Println("2. Confirmar reserva (de uma busca recente)")
		fmt.Println("3. Consultar minhas reservas")
		fmt.Println("4. Cancelar reserva")
		fmt.Println("5. Encerrar conexão")

		switch lerInt(leitor, "Escolha: ") {
		case 1:
			buscarItinerarios(conexao, leitor)
		case 2:
			confirmarReserva(conexao, leitor)
		case 3:
			consultarReservas(conexao)
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

	dados, _ := json.Marshal(protocolo.BuscarItinerariosDados{Origem: origem, Destino: destino, Data: data})

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "buscar_itinerarios", IDRequisicao: novoIDRequisicao(), Dados: dados})
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
	ultimaBusca = resultado.Itinerarios

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

func confirmarReserva(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	if len(ultimaBusca) == 0 {
		fmt.Println("Busque um itinerário primeiro (opção 1).")
		return
	}

	indice := lerInt(leitor, fmt.Sprintf("Qual itinerário confirmar? (0 a %d): ", len(ultimaBusca)-1))
	if indice < 0 || indice >= len(ultimaBusca) {
		fmt.Println("Índice inválido.")
		return
	}

	dados, _ := json.Marshal(protocolo.ConfirmarReservaDados{Trechos: ultimaBusca[indice].Trechos})

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "confirmar_reserva", IDRequisicao: novoIDRequisicao(), Dados: dados})
	if err != nil {
		fmt.Println("erro de rede:", err)
		return
	}
	if resp.Status != "ok" {
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

func consultarReservas(conexao *clientenet.Conexao) {
	dados, _ := json.Marshal(protocolo.ConsultarReservasDados{})

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "consultar_reservas", IDRequisicao: novoIDRequisicao(), Dados: dados})
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

	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "cancelar_reserva", IDRequisicao: novoIDRequisicao(), Dados: dados})
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

// ============================================================================
// Helpers de entrada/saida
// ============================================================================

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

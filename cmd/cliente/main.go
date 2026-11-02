package main

// Cliente unico do VaiJunto: pede cadastro ou login (com o papel —
// motorista ou passageiro — escolhido no cadastro e devolvido pelo
// login), e a partir dai mostra o menu certo.

import (
	"bufio"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"vaijunto/internal/clientenet"
	"vaijunto/internal/protocolo"
)

var sequenciaFallback uint64

// estadoPersistidoCliente mantém a chave da última tentativa de reserva
// por assinatura do passageiro + itinerário. A chave sobrevive ao reinício
// do terminal, permitindo repetir uma solicitação cujo resultado foi perdido.
type estadoPersistidoCliente struct {
	ChavesPendentes     map[string]string `json:"chaves_pendentes"`
	ServidoresPendentes map[string]string `json:"servidores_pendentes,omitempty"`
}

var estadoCliente = estadoPersistidoCliente{
	ChavesPendentes:     make(map[string]string),
	ServidoresPendentes: make(map[string]string),
}

// ultimaBusca guarda os itinerarios da ultima busca do passageiro, para
// "confirmar reserva" poder referenciar um deles por indice. E so
// estado de sessao do terminal, nao tem nada a ver com o servidor.
var ultimaBusca []protocolo.ItinerarioDados

func main() {
	if err := carregarEstadoCliente(); err != nil {
		// Falhar fechado evita iniciar uma nova tentativa com outra chave
		// quando o arquivo anterior não pôde ser lido; isso poderia duplicar
		// uma reserva cujo resultado ficou desconhecido.
		log.Fatalf("nao foi possivel carregar estado de idempotencia do cliente: %v", err)
	}

	// O cliente vai se conectar a um servidor, cujo endereco (host:porta) pode ser passado como argumento de linha de comando. Se nao for, usamos
	// localhost:8080.
	endereco := flag.String("servidor", "localhost:8080", "endereco do servidor (host:porta)")
	flag.Parse()

	// Conecta ao servidor
	conexao, err := clientenet.Conectar(*endereco)
	if err != nil {
		log.Fatal(err)
	}
	defer conexao.Fechar()

	fmt.Printf("Conectado ao servidor: %s\n", *endereco)

	// Testa a comunicação antes de iniciar o cadastro ou login.
	pingCliente(conexao)

	// Pede ao usuario se quer se cadastrar ou autenticar, e chama a funcao certa.
	leitor := bufio.NewReader(os.Stdin) // para ler do terminal

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

func pingCliente(conexao *clientenet.Conexao) {
	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "ping", IDRequisicao: novoIDRequisicao()})
	if err != nil {
		log.Fatal("erro de rede ao enviar ping: ", err)
	}

	if resp.Status != "ok" {
		log.Fatal("ping falhou: ", resp.Motivo)
	}

	fmt.Printf("PING: OK \n Dados: %s\n", resp.Dados)

}

// cadastrarCliente pede ao usuario o papel (motorista ou passageiro), nome de usuario e senha, envia para o servidor e trata a resposta.
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

		// Envia a requisicao de cadastro para o servidor e trata a resposta.
		resp, err := conexao.Enviar(protocolo.Requisicao{
			Tipo:         "cadastro",
			IDRequisicao: novoIDRequisicao(),
			Dados:        dados,
		})
		if err != nil {
			log.Fatal("erro de rede ao cadastrar: ", err)
		}

		// O servidor devolve "ok" se o cadastro foi bem sucedido, ou "erro" com um motivo caso contrario.
		if resp.Status == "ok" {
			fmt.Println("\nCadastro realizado com sucesso!")
			// Cadastrar nao loga automaticamente (o servidor so cria o
			// usuario) — o proprio usuario ja sabe seu role, escolhido
			// agora mesmo, entao pulamos direto pro menu certo.
			menu(conexao, leitor, role, usuario)
			return
		}
		fmt.Printf("\nFalha no cadastro: %s\nTente novamente.\n\n", resp.Motivo)
	}
}

// loginCliente pede ao usuario nome de usuario e senha, envia para o servidor e trata a resposta.
func loginCliente(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	for {
		usuario := lerLinha(leitor, "Nome de usuário: ")
		senha := lerLinha(leitor, "Senha: ")

		// Serializa os dados de login em JSON
		dados, _ := json.Marshal(protocolo.LoginDados{Usuario: usuario, Senha: senha})

		// Envia a requisicao de login para o servidor e trata a resposta.
		resp, err := conexao.Enviar(protocolo.Requisicao{
			Tipo:         "login",
			IDRequisicao: novoIDRequisicao(),
			Dados:        dados,
		})
		if err != nil {
			log.Fatal("erro de rede ao logar: ", err)
		}

		// O servidor devolve "ok" se o login foi bem sucedido, ou "erro" com um motivo caso contrario.
		if resp.Status != "ok" {
			fmt.Printf("\nFalha no login: %s\nTente novamente.\n\n", resp.Motivo)
			continue
		}

		var loginResp protocolo.LoginResposta
		if err := json.Unmarshal(resp.Dados, &loginResp); err != nil {
			log.Fatal("resposta de login inesperada: ", err)
		}
		fmt.Println("\n" + loginResp.Mensagem)
		menu(conexao, leitor, loginResp.Role, usuario)
		return
	}
}

// menu direciona para o menu certo conforme o Role devolvido pelo
// servidor (nunca escolhido livremente pelo cliente depois do login).
func menu(conexao *clientenet.Conexao, leitor *bufio.Reader, role, usuario string) {
	switch role {
	case "m":
		menuMotorista(conexao, leitor)
	case "p":
		menuPassageiro(conexao, leitor, usuario)
	default:
		fmt.Println("Role desconhecido:", role)
	}
}

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
	// Coleta os dados da carona do usuário.

	rotaTexto := lerLinha(leitor, "Rota (cidades separadas por vírgula, ex: Salvador,Feira de Santana): ")
	rota := strings.Split(rotaTexto, ",")
	for i := range rota {
		rota[i] = strings.TrimSpace(rota[i])
	}

	capacidade := lerInt(leitor, "Capacidade de assentos: ")
	preco := lerInt(leitor, "Preço: ")
	data := lerLinha(leitor, "Data (AAAA-MM-DD): ")

	// Serializa os dados da carona em JSON
	dados, _ := json.Marshal(protocolo.PublicarCaronaDados{Rota: rota, Capacidade: capacidade, Preco: preco, Data: data})

	// Envia a requisicao de publicar carona para o servidor e trata a resposta.
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

func menuPassageiro(conexao *clientenet.Conexao, leitor *bufio.Reader, usuario string) {
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
			confirmarReserva(conexao, leitor, usuario)
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
	passageiros := lerInt(leitor, "Numero de passageiros: ")

	dados, err := json.Marshal(protocolo.BuscarItinerariosDados{
		Origem:         origem,
		Destino:        destino,
		Data:           data,
		NumPassageiros: passageiros,
	})
	if err != nil {
		fmt.Println("erro ao preparar busca:", err)
		return
	}

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
			partes = append(partes, fmt.Sprintf("%s->%s (empres %s, carona %s)", t.Origem, t.Destino, t.EmpresaID, t.IDCarona))
		}
		fmt.Printf("  [%d] %s\n", i, strings.Join(partes, "  |  "))
	}

	if !resultado.BuscaCompleta {
		fmt.Printf(
			"Aviso: busca parcial. Empresas indisponiveis: %v\n",
			resultado.EmpresasIndisponiveis,
		)
	}
}

func confirmarReserva(conexao *clientenet.Conexao, leitor *bufio.Reader, usuario string) {
	// Verifica se há uma busca recente para confirmar reserva. Se não houver, informa o usuário e retorna.
	if len(ultimaBusca) == 0 {
		fmt.Println("Busque um itinerário primeiro (opção 1).")
		return
	}

	// Pede ao usuário o índice do itinerário que deseja confirmar reserva, e valida se o índice é válido.
	indice := lerInt(leitor, fmt.Sprintf("Qual itinerário confirmar? (0 a %d): ", len(ultimaBusca)-1))
	if indice < 0 || indice >= len(ultimaBusca) {
		fmt.Println("Índice inválido.")
		return
	}

	// A chave e persistida antes do envio. Assim, se a conexao cair ou o
	// cliente reiniciar, a mesma selecao do mesmo passageiro pode ser repetida.
	dados, err := json.Marshal(protocolo.ConfirmarReservaDados{Trechos: ultimaBusca[indice].Trechos})
	if err != nil {
		fmt.Println("erro ao preparar reserva:", err)
		return
	}
	chaveSolicitacao := assinaturaReservaCliente(usuario, dados)
	idRequisicao, err := obterChaveReservaPendente(chaveSolicitacao, conexao.Endereco())
	if err != nil {
		fmt.Println("nao foi possivel persistir a chave de idempotencia:", err)
		return
	}

	// Em caso de timeout, a chave permanece armazenada para uma nova tentativa.
	resp, err := conexao.Enviar(protocolo.Requisicao{Tipo: "confirmar_reserva", IDRequisicao: idRequisicao, Dados: dados})
	if err != nil {
		fmt.Println("resultado da reserva desconhecido; repita a mesma selecao para consultar a mesma transacao:", err)
		return
	}
	if resp.Status != "ok" {
		fmt.Println("Não foi possível confirmar:", resp.Motivo)
		return
	}

	// A resposta agora descreve a transação coordenada e as reservas locais
	// criadas por cada empresa participante.
	var resultado protocolo.CoordenarReservaResposta
	if err := json.Unmarshal(resp.Dados, &resultado); err != nil || resultado.TransacaoID == "" {
		// Compatibilidade com servidores que ainda devolvem uma reserva local.
		var reserva protocolo.ReservaResposta
		if erro := json.Unmarshal(resp.Dados, &reserva); erro != nil {
			fmt.Println("resposta inesperada do servidor:", erro)
			return
		}
		fmt.Printf("Reserva local confirmada! ID=%d\n", reserva.ID)
		removerChaveReservaPendente(chaveSolicitacao)
		return
	}

	switch resultado.Estado {
	case "CONFIRMADA":
		fmt.Printf("Reserva distribuida confirmada! Transacao=%s\n", resultado.TransacaoID)
		for _, reserva := range resultado.Reservas {
			fmt.Printf("  Empresa=%s reserva local ID=%d\n", reserva.EmpresaID, reserva.ID)
		}
		removerChaveReservaPendente(chaveSolicitacao)
	case "ABORTADA":
		fmt.Printf("Reserva abortada. Transacao=%s. Motivo: %s\n", resultado.TransacaoID, resultado.Mensagem)
		removerChaveReservaPendente(chaveSolicitacao)
	case "PENDENTE_CONFIRMAR", "PENDENTE_ABORTAR", "PREPARANDO":
		fmt.Printf("Transacao %s ainda esta pendente (%s). A reserva NAO deve ser considerada confirmada.\n", resultado.TransacaoID, resultado.Estado)
		if len(resultado.EmpresasPendentes) > 0 {
			fmt.Printf("Empresas sem ACK: %v\n", resultado.EmpresasPendentes)
		}
	default:
		fmt.Printf("Estado da transacao %s: %s (%s)\n", resultado.TransacaoID, resultado.Estado, resultado.Mensagem)
	}
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
		fmt.Printf("  [Empresa %s | Reserva %d | Transacao %s] %s\n", r.EmpresaID, r.ID, r.TransacaoID, strings.Join(partes, "  |  "))
	}
}

func cancelarReserva(conexao *clientenet.Conexao, leitor *bufio.Reader) {
	empresaID := lerLinha(leitor, "Empresa proprietaria da reserva (conforme consulta): ")
	id := lerInt(leitor, "ID local da reserva: ")
	transacaoID := lerLinha(leitor, "ID da transacao distribuida (vazio para reserva local antiga): ")
	dados, _ := json.Marshal(protocolo.CancelarReservaDados{
		IDReserva:   id,
		EmpresaID:   empresaID,
		TransacaoID: transacaoID,
	})

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

func caminhoEstadoCliente() string {
	if caminho := strings.TrimSpace(os.Getenv("VAIJUNTO_CLIENTE_ESTADO")); caminho != "" {
		return caminho
	}
	return filepath.Join("dados", "cliente_estado.json")
}

func carregarEstadoCliente() error {
	conteudo, err := os.ReadFile(caminhoEstadoCliente())
	if os.IsNotExist(err) {
		estadoCliente = estadoPersistidoCliente{
			ChavesPendentes:     make(map[string]string),
			ServidoresPendentes: make(map[string]string),
		}
		return nil
	}
	if err != nil {
		return err
	}
	var carregado estadoPersistidoCliente
	if err := json.Unmarshal(conteudo, &carregado); err != nil {
		return fmt.Errorf("estado local do cliente invalido: %w", err)
	}
	if carregado.ChavesPendentes == nil {
		carregado.ChavesPendentes = make(map[string]string)
	}
	if carregado.ServidoresPendentes == nil {
		carregado.ServidoresPendentes = make(map[string]string)
	}
	estadoCliente = carregado
	return nil
}

func salvarEstadoCliente() error {
	if estadoCliente.ChavesPendentes == nil {
		estadoCliente.ChavesPendentes = make(map[string]string)
	}
	if estadoCliente.ServidoresPendentes == nil {
		estadoCliente.ServidoresPendentes = make(map[string]string)
	}
	conteudo, err := json.MarshalIndent(estadoCliente, "", "  ")
	if err != nil {
		return err
	}
	caminho := caminhoEstadoCliente()
	diretorio := filepath.Dir(caminho)
	if err := os.MkdirAll(diretorio, 0700); err != nil {
		return err
	}
	arquivo, err := os.CreateTemp(diretorio, ".cliente-estado-*.tmp")
	if err != nil {
		return err
	}
	nomeTemp := arquivo.Name()
	defer os.Remove(nomeTemp)
	if err := arquivo.Chmod(0600); err != nil {
		_ = arquivo.Close()
		return err
	}
	if _, err := arquivo.Write(conteudo); err != nil {
		_ = arquivo.Close()
		return err
	}
	if err := arquivo.Sync(); err != nil {
		_ = arquivo.Close()
		return err
	}
	if err := arquivo.Close(); err != nil {
		return err
	}
	return os.Rename(nomeTemp, caminho)
}

func assinaturaReservaCliente(usuario string, dados []byte) string {
	entrada := append([]byte(strings.TrimSpace(usuario)+"\n"), dados...)
	hash := sha256.Sum256(entrada)
	return hex.EncodeToString(hash[:])
}

func obterChaveReservaPendente(assinatura, servidor string) (string, error) {
	servidor = strings.TrimSpace(servidor)
	if id, existe := estadoCliente.ChavesPendentes[assinatura]; existe {
		servidorAnterior := estadoCliente.ServidoresPendentes[assinatura]
		if servidorAnterior != "" && servidor != "" && servidorAnterior != servidor {
			return "", fmt.Errorf(
				"esta reserva ainda possui uma tentativa pendente no servidor %s; reconecte-se a esse mesmo servidor para repetir a solicitacao",
				servidorAnterior,
			)
		}
		if servidorAnterior == "" && servidor != "" {
			estadoCliente.ServidoresPendentes[assinatura] = servidor
			if err := salvarEstadoCliente(); err != nil {
				delete(estadoCliente.ServidoresPendentes, assinatura)
				return "", err
			}
		}
		return id, nil
	}
	id := novoIDRequisicao()
	estadoCliente.ChavesPendentes[assinatura] = id
	if servidor != "" {
		estadoCliente.ServidoresPendentes[assinatura] = servidor
	}
	if err := salvarEstadoCliente(); err != nil {
		delete(estadoCliente.ChavesPendentes, assinatura)
		delete(estadoCliente.ServidoresPendentes, assinatura)
		return "", err
	}
	return id, nil
}

func removerChaveReservaPendente(assinatura string) {
	delete(estadoCliente.ChavesPendentes, assinatura)
	delete(estadoCliente.ServidoresPendentes, assinatura)
	if err := salvarEstadoCliente(); err != nil {
		fmt.Printf("aviso: a reserva foi concluida, mas nao foi possivel limpar a chave local: %v\n", err)
	}
}

// novoIDRequisicao gera um identificador praticamente unico entre processos
// e reinicializacoes. Para confirmar_reserva ele tambem funciona como chave
// de idempotencia, por isso nao pode reiniciar em "1" a cada cliente.
func novoIDRequisicao() string {
	var bytes [16]byte
	if _, err := cryptorand.Read(bytes[:]); err == nil {
		bytes[6] = (bytes[6] & 0x0f) | 0x40
		bytes[8] = (bytes[8] & 0x3f) | 0x80
		return hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("%x-%d", time.Now().UnixNano(), atomic.AddUint64(&sequenciaFallback, 1))
}

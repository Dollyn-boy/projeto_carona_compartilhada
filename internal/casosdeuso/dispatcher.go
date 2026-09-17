// Package casosdeuso é o roteador de operações do servidor — a camada
// "Casos de uso" da arquitetura. Olha o tipo de uma mensagem já
// parseada pelo pacote protocolo e decide qual caso de uso executar,
// chamando o pacote estado para ler ou alterar os dados.
package casosdeuso

import (
	"encoding/json"
	"time"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// Despachar decide o que fazer com uma requisição já parseada, com base
// em req.Tipo, e devolve a resposta correspondente.
//

func Despachar(repo *estado.Repositorio, sessao *Sessao, req protocolo.Requisicao) protocolo.Resposta {

	limiteInatividade := 30 * time.Minute
	if sessao.Autenticado {
		if time.Since(sessao.UltimaAtividade) > limiteInatividade {
			// Tempo estourou: desloga o usuário forçadamente
			sessao.Autenticado = false
			sessao.Usuario = ""
			return erro(req, "Sessão expirada por inatividade. Faça login novamente.")
		}
		// Atualiza o relógio a cada requisição válida
		sessao.UltimaAtividade = time.Now()
	}
	switch req.Tipo {
	case "ping":
		return tratarPing(req)
	case "cadastro":
		return tratarCadastro(repo, sessao, req)
	case "login":
		// Passamos o ponteiro da sessão para o login poder alterá-la!
		return tratarLogin(repo, sessao, req)
	case "logout":
		return tratarLogout(sessao, req)
	}

	// 3. BARREIRA DE AUTENTICAÇÃO
	// Se chegou aqui e não está autenticado, é porque tentou acessar uma rota privada.
	if !sessao.Autenticado {
		return erro(req, "Acesso negado: faça login primeiro")
	}

	// 4. ROTAS PRIVADAS (Somente autenticados passam daqui)
	switch req.Tipo {
	case "publicar_carona":
		// Sugestão: Passe o sessao.Usuario para a função saber QUEM está publicando.
		// Assim você não confia no ID que vem do payload do cliente, que pode ser forjado.
		return tratarPublicarCarona(repo, sessao.Usuario, req)

	case "consultar_caronas":
		return tratarConsultarCaronas(repo, sessao.Usuario, req)

	case "cancelar_carona":
		return tratarCancelarCarona(repo, sessao.Usuario, req)

	case "buscar_itinerarios":
		return tratarBuscarItinerarios(repo, req)

	case "confirmar_reserva":
		return tratarConfirmarReserva(repo, sessao.Usuario, req)

	case "consultar_reservas":
		return tratarConsultarReservas(repo, sessao.Usuario, req)

	case "cancelar_reserva":
		return tratarCancelarReserva(repo, sessao.Usuario, req)

	default:
		return erro(req, "tipo de operacao desconhecido ou invalido: "+req.Tipo)
	}
}


func tratarLogin(repo *estado.Repositorio, sessao *Sessao, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.LoginDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	autenticado, role, err := repo.AutenticarUsuario(dados.Usuario, dados.Senha)
	if err != nil || !autenticado {
		return erro(req, "usuario ou senha invalidos")
	}

	sessao.Autenticado = true
	sessao.Usuario = dados.Usuario
	sessao.Role = role
	sessao.UltimaAtividade = time.Now()

	return ok(req, protocolo.LoginResposta{
		Mensagem: "bem-vindo, " + dados.Usuario,
		Role:     role,
	})
}

func tratarLogout(sessao *Sessao, req protocolo.Requisicao) protocolo.Resposta {
	if !sessao.Autenticado {
		return erro(req, "Você já está desconectado")
	}

	// Limpa a sessão
	sessao.Autenticado = false
	sessao.Usuario = ""

	return ok(req, "logout ok")
}

func tratarCadastro(repo *estado.Repositorio, sessao *Sessao, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.CadastroDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	if dados.Usuario == "" {
		return erro(req, "nome de usuario nao pode ser vazio")
	}
	if dados.Role != "m" && dados.Role != "p" {
		return erro(req, "role deve ser 'm' (motorista) ou 'p' (passageiro)")
	}

	if err := repo.CadastrarUsuario(dados.Usuario, dados.Senha, dados.Role); err != nil {
		return erro(req, err.Error()) 
	}

	sessao.Autenticado = true
	sessao.Usuario = dados.Usuario
	sessao.Role = dados.Role
	sessao.UltimaAtividade = time.Now()

	return ok(req, protocolo.LoginResposta{
		Mensagem: "cadastro realizado com sucesso, bem-vindo " + dados.Usuario,
		Role:     dados.Role,
	})
}

// Helpers compartilhados por motorista.go e passageiro.go (mesmo pacote)

func ok(req protocolo.Requisicao, dados any) protocolo.Resposta {
	bytes, err := json.Marshal(dados)
	if err != nil {
		return erro(req, "falha ao montar resposta: "+err.Error())
	}
	return protocolo.Resposta{Status: "ok", IDRequisicao: req.IDRequisicao, Dados: bytes}
}

// erro monta uma Resposta de erro com o motivo informado.
func erro(req protocolo.Requisicao, motivo string) protocolo.Resposta {
	return protocolo.Resposta{Status: "erro", IDRequisicao: req.IDRequisicao, Motivo: motivo}
}

// caronaParaResposta converte o tipo interno dominio.Carona para o tipo
// de protocolo CaronaResposta (serializavel, sem expor dominio direto
// no fio).
func caronaParaResposta(c dominio.Carona) protocolo.CaronaResposta {
	rota := make([]string, len(c.Rota))
	for i, cidade := range c.Rota {
		rota[i] = string(cidade)
	}
	return protocolo.CaronaResposta{
		ID:         c.Id,
		Motorista:  c.Motorista,
		Rota:       rota,
		Capacidade: c.Capacidade,
		Preco:      c.Preco,
		Data:       c.Data.Format("2006-01-02"),
	}
}

func tratarPing(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal("pong")
	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
	}
}

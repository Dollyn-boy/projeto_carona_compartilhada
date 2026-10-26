package casosdeuso

import (
	"encoding/json"
	"time"

	"vaijunto/internal/consorcio"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// Despachar decide o que fazer com uma requisição já parseada,
// com base em req.Tipo, e devolve a resposta correspondente.
//
// Mantém compatibilidade com chamadas que ainda não utilizam
// o serviço de busca distribuída.
func Despachar(
	repo *estado.Repositorio,
	sessao *Sessao,
	req protocolo.Requisicao,
) protocolo.Resposta {
	return despachar(repo, sessao, req, nil)
}

// DespacharComBusca encaminha as requisições para o dispatcher,
// disponibilizando o serviço de busca distribuída.
func DespacharComBusca(
	repo *estado.Repositorio,
	sessao *Sessao,
	req protocolo.Requisicao,
	busca *consorcio.BuscaDistribuida,
) protocolo.Resposta {
	return despachar(repo, sessao, req, busca)
}

// despachar centraliza o roteamento das operações.
// O parâmetro busca pode ser nil quando a busca distribuída
// ainda não estiver configurada.
func despachar(
	repo *estado.Repositorio,
	sessao *Sessao,
	req protocolo.Requisicao,
	busca *consorcio.BuscaDistribuida,
) protocolo.Resposta {
	limiteInatividade := 30 * time.Minute

	if sessao.Autenticado {
		// Se o usuário está autenticado, verifica se a sessão
		// expirou por inatividade.
		if time.Since(sessao.UltimaAtividade) > limiteInatividade {
			// Desloga o usuário forçadamente.
			sessao.Autenticado = false
			sessao.Usuario = ""
			sessao.Role = ""

			return erro(
				req,
				"Sessão expirada por inatividade. Faça login novamente.",
			)
		}

		// Atualiza o relógio a cada requisição recebida.
		sessao.UltimaAtividade = time.Now()
	}

	// 1. Rotas públicas.
	switch req.Tipo {
	case "ping":
		return tratarPing(req)

	case "cadastro":
		return tratarCadastro(repo, sessao, req)

	case "login":
		return tratarLogin(repo, sessao, req)

	case "logout":
		return tratarLogout(sessao, req)
	}

	// 2. Barreira de autenticação.
	// Todas as operações restantes exigem uma sessão autenticada.
	if !sessao.Autenticado {
		return erro(req, "Acesso negado: faça login primeiro")
	}

	// 3. Rotas privadas.
	switch req.Tipo {
	case "publicar_carona":
		// Utiliza o usuário autenticado, e não um ID fornecido
		// livremente pelo cliente.
		return tratarPublicarCarona(repo, sessao.Usuario, req)

	case "consultar_caronas":
		return tratarConsultarCaronas(repo, sessao.Usuario, req)

	case "cancelar_carona":
		return tratarCancelarCarona(repo, sessao.Usuario, req)

	case "buscar_itinerarios":
		// Se o serviço distribuído estiver configurado, utiliza-o.
		// Caso contrário, mantém o comportamento local existente.

		return tratarBuscarItinerarios(repo, req, busca)

	case "confirmar_reserva":
		return tratarConfirmarReserva(repo, sessao.Usuario, req)

	case "consultar_reservas":
		return tratarConsultarReservas(repo, sessao.Usuario, req)

	case "cancelar_reserva":
		return tratarCancelarReserva(repo, sessao.Usuario, req)

	default:
		return erro(
			req,
			"tipo de operacao desconhecido ou invalido: "+req.Tipo,
		)
	}
}

// tratarLogin valida as credenciais e atualiza a sessão.
func tratarLogin(
	repo *estado.Repositorio,
	sessao *Sessao,
	req protocolo.Requisicao,
) protocolo.Resposta {
	var dados protocolo.LoginDados

	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	autenticado, role, err := repo.AutenticarUsuario(
		dados.Usuario,
		dados.Senha,
	)
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

// tratarLogout encerra a sessão atual.
func tratarLogout(
	sessao *Sessao,
	req protocolo.Requisicao,
) protocolo.Resposta {
	if !sessao.Autenticado {
		return erro(req, "Você já está desconectado")
	}

	sessao.Autenticado = false
	sessao.Usuario = ""
	sessao.Role = ""

	return ok(req, "logout ok")
}

// tratarCadastro valida os dados e cadastra um usuário.
// Em caso de sucesso, autentica o usuário na sessão.
func tratarCadastro(
	repo *estado.Repositorio,
	sessao *Sessao,
	req protocolo.Requisicao,
) protocolo.Resposta {
	var dados protocolo.CadastroDados

	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	if dados.Usuario == "" {
		return erro(req, "nome de usuario nao pode ser vazio")
	}

	if dados.Role != "m" && dados.Role != "p" {
		return erro(
			req,
			"role deve ser 'm' (motorista) ou 'p' (passageiro)",
		)
	}

	if err := repo.CadastrarUsuario(
		dados.Usuario,
		dados.Senha,
		dados.Role,
	); err != nil {
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

// Helpers compartilhados por motorista.go e passageiro.go.

func ok(
	req protocolo.Requisicao,
	dados any,
) protocolo.Resposta {
	bytes, err := json.Marshal(dados)
	if err != nil {
		return erro(req, "falha ao montar resposta: "+err.Error())
	}

	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        bytes,
	}
}

// erro monta uma resposta de erro com o motivo informado.
func erro(
	req protocolo.Requisicao,
	motivo string,
) protocolo.Resposta {
	return protocolo.Resposta{
		Status:       "erro",
		IDRequisicao: req.IDRequisicao,
		Motivo:       motivo,
	}
}

// caronaParaResposta converte dominio.Carona para o tipo
// serializável protocolo.CaronaResposta.
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

// tratarPing responde "pong" a uma requisição "ping".
func tratarPing(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal("pong")

	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
	}
}

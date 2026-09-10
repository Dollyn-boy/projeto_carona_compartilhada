// Package casosdeuso é o roteador de operações do servidor — a camada
// "Casos de uso" da arquitetura. Olha o tipo de uma mensagem já
// parseada pelo pacote protocolo e decide qual caso de uso executar,
// chamando o pacote estado para ler ou alterar os dados.
package casosdeuso

import (
	"encoding/json"
	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// Despachar decide o que fazer com uma requisição já parseada, com base
// em req.Tipo, e devolve a resposta correspondente.
//
// TODO: à medida que internal/estado for existindo de verdade, troque os
// handlers de motorista.go e passageiro.go para chamá-lo, e registre-os
// aqui. Quando login existir, esta função provavelmente vai precisar
// receber também uma sessão da conexão (ver internal/rede) para saber
// quem está autenticado.
//
// Por enquanto só existe "ping" — serve para validar a tubulação
// completa (rede + protocolo + roteamento) antes de qualquer lógica de
// negócio real.
// Despachar decide o que fazer com uma requisição já parseada, com base
// em req.Tipo, e devolve a resposta correspondente.
//
// CORRECAO: agora recebe *estado.Repositorio como parâmetro — antes não
// havia NENHUMA forma de um handler chegar até o estado real do
// servidor (nem internal/rede nem cmd/servidor criavam um Repositorio).
// Esse repo é criado UMA VEZ em cmd/servidor/main.go e repassado através
// de rede.Iniciar -> tratarConexao -> aqui, sempre o MESMO ponteiro —
// é por isso que uma carona publicada por uma conexão aparece pra
// consultas feitas por outra conexão.
//
// TODO: quando login/cadastro autenticarem de verdade, esta função
// provavelmente vai precisar receber também uma sessão da conexão (ver
// internal/rede) para saber quem está autenticado.

func Despachar(repo *estado.Repositorio, req protocolo.Requisicao) protocolo.Resposta {
	switch req.Tipo {
	case "ping":
		return tratarPing(req)
	case "login":
		return tratarLogin(req)
	case "cadastro":
		return tratarCadastro(req)
	case "publicar_carona":
		return tratarPublicarCarona(repo, req)
	case "consultar_caronas":
		return tratarConsultarCaronas(repo, req)
	case "cancelar_carona":
		return tratarCancelarCarona(repo, req)
	case "buscar_itinerarios":
		return tratarBuscarItinerarios(repo, req)
	case "confirmar_reserva":
		return tratarConfirmarReserva(repo, req)
	case "consultar_reservas":
		return tratarConsultarReservas(repo, req)
	case "cancelar_reserva":
		return tratarCancelarReserva(repo, req)
	default:
		return protocolo.Resposta{
			Status:       "erro",
			IDRequisicao: req.IDRequisicao,
			Motivo:       "tipo de operacao desconhecido: " + req.Tipo,
		}
	}
}

// ============================================================================
// Helpers compartilhados por motorista.go e passageiro.go (mesmo pacote)
// ============================================================================

// ok monta uma Resposta de sucesso, serializando "dados" (qualquer
// struct de protocolo, ex: protocolo.CaronaResposta) para JSON.
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

func tratarLogin(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal("login ok")
	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
	}
}

func tratarCadastro(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal("cadastro ok")
	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
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

// Package casosdeuso é o roteador de operações do servidor — a camada
// "Casos de uso" da arquitetura. Olha o tipo de uma mensagem já
// parseada pelo pacote protocolo e decide qual caso de uso executar,
// chamando o pacote estado para ler ou alterar os dados.
package casosdeuso

import (
	"encoding/json"

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
func Despachar(req protocolo.Requisicao) protocolo.Resposta {
	switch req.Tipo {
	case "ping":
		return tratarPing(req)
	case "login":
		return tratarLogin(req)
	case "cadastro":
		return tratarCadastro(req)
	case "publicar_carona":
		return tratarPublicarCarona(req)
	case "consultar_caronas":
		return tratarConsultarCaronas(req)
	case "cancelar_carona":
		return tratarCancelarCarona(req)
	case "buscar_itinerarios":
		return tratarBuscarItinerarios(req)
	case "confirmar_reserva":
		return tratarConfirmarReserva(req)
	case "consultar_reservas":
		return tratarConsultarReservas(req)
	case "cancelar_reserva":
		return tratarCancelarReserva(req)
	default:
		return protocolo.Resposta{
			Status:       "erro",
			IDRequisicao: req.IDRequisicao,
			Motivo:       "tipo de operacao desconhecido: " + req.Tipo,
		}
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

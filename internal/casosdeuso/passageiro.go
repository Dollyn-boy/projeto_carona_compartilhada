package casosdeuso

import (
	"encoding/json"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// itinerarioParaDados converte um dominio.Itinerario (achado pela busca)
// para o formato do protocolo, pra poder ser serializado e enviado ao
// cliente.
func itinerarioParaDados(it dominio.Itinerario) protocolo.ItinerarioDados {
	trechos := make([]protocolo.TrechoDados, len(it.Passos))
	for i, t := range it.Passos {
		trechos[i] = protocolo.TrechoDados{
			IDCarona: t.IDCarona,
			Indice:   t.Indice,
			Origem:   string(t.Origem),
			Destino:  string(t.Destino),
		}
	}
	return protocolo.ItinerarioDados{Trechos: trechos}
}

// dadosParaItinerario faz o caminho inverso: o cliente reenvia o
// itinerario que escolheu (recebido antes numa busca) como
// []TrechoDados, e aqui reconstruimos o dominio.Itinerario que
// repo.ConfirmarReserva espera.
func dadosParaItinerario(trechos []protocolo.TrechoDados) dominio.Itinerario {
	passos := make([]dominio.Trecho, len(trechos))
	for i, t := range trechos {
		passos[i] = dominio.Trecho{
			IDCarona: t.IDCarona,
			Indice:   t.Indice,
			Origem:   dominio.Cidade(t.Origem),
			Destino:  dominio.Cidade(t.Destino),
		}
	}
	return dominio.Itinerario{Passos: passos}
}

// reservaParaResposta converte um dominio.Reserva para o formato do
// protocolo — usado tanto por ConfirmarReserva quanto por
// ConsultarReservas.
func reservaParaResposta(r dominio.Reserva) protocolo.ReservaResposta {
	trechos := make([]protocolo.TrechoDados, len(r.Itens))
	for i, item := range r.Itens {
		trechos[i] = protocolo.TrechoDados{
			IDCarona: item.Trecho.IDCarona,
			Indice:   item.Trecho.Indice,
			Origem:   string(item.Trecho.Origem),
			Destino:  string(item.Trecho.Destino),
		}
	}
	return protocolo.ReservaResposta{
		ID:         r.Id,
		Passageiro: r.Passageiro,
		Trechos:    trechos,
	}
}

// tratarBuscarItinerarios: chama repo.BuscarItinerarios (que por baixo
// usa dominio.BuscarTodosItinerarios) e devolve TODOS os itinerarios
// encontrados — o cliente escolhe qual confirmar depois.
//
// NOTA (gap conhecido): dados.Data e dados.NumPassageiros ainda nao sao
// usados aqui, porque repo.BuscarItinerarios nao filtra por data (ver
// carona.go). Se adicionar isso depois, e aqui que o filtro entra.
func tratarBuscarItinerarios(repo *estado.Repositorio, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.BuscarItinerariosDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	itinerarios, err := repo.BuscarItinerarios(dominio.Cidade(dados.Origem), dominio.Cidade(dados.Destino))
	if err != nil {
		return erro(req, err.Error())
	}

	var resposta protocolo.BuscarItinerariosResposta
	for _, it := range itinerarios {
		resposta.Itinerarios = append(resposta.Itinerarios, itinerarioParaDados(it))
	}

	return ok(req, resposta)
}

// tratarConfirmarReserva agora usa o usuario da sessão
func tratarConfirmarReserva(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.ConfirmarReservaDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	itinerario := dadosParaItinerario(dados.Trechos)

	// SEGURANÇA: Usamos usuario em vez de dados.Passageiro
	reserva, err := repo.ConfirmarReserva(usuario, itinerario)
	if err != nil {
		return erro(req, err.Error())
	}

	return ok(req, reservaParaResposta(reserva))
}

// tratarConsultarReservas agora busca reservas atreladas à sessão
func tratarConsultarReservas(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
	// SEGURANÇA: Busca apenas as reservas do usuário logado
	reservas, err := repo.ConsultarReservas(usuario)
	if err != nil {
		return erro(req, err.Error())
	}

	var resposta protocolo.ConsultarReservasResposta
	for _, r := range reservas {
		resposta.Reservas = append(resposta.Reservas, reservaParaResposta(r))
	}

	return ok(req, resposta)
}

func tratarCancelarReserva(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.CancelarReservaDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	// IDEALMENTE: Atualize para repo.CancelarReserva(dados.IDReserva, usuario)
	if err := repo.CancelarReserva(dados.IDReserva, usuario); err != nil {
		return erro(req, err.Error())
	}

	return ok(req, map[string]string{"mensagem": "reserva cancelada"})
}

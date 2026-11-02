package casosdeuso

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"vaijunto/internal/consorcio"
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
			EmpresaID: t.EmpresaID,
			IDCarona:  t.IDCarona,
			Indice:    t.Indice,
			Origem:    string(t.Origem),
			Destino:   string(t.Destino),
		}
	}
	return protocolo.ItinerarioDados{Trechos: trechos}
}

// dadosParaItinerario faz o caminho inverso: o cliente reenvia o
// itinerario que escolheu (recebido antes numa busca) como
// []TrechoDados, e aqui reconstruimos o dominio.Itinerario que
// repo.ConfirmarReserva espera.
func dadosParaItinerario(trechos []protocolo.TrechoDados) dominio.Itinerario {
	passos := make([]dominio.Trecho, len(trechos)) // cria slice de tamanho correto

	// Para cada trecho recebido do cliente, converte para dominio.Trecho e coloca no slice passos.
	for i, t := range trechos {
		passos[i] = dominio.Trecho{
			EmpresaID: t.EmpresaID,
			IDCarona:  t.IDCarona,
			Indice:    t.Indice,
			Origem:    dominio.Cidade(t.Origem),
			Destino:   dominio.Cidade(t.Destino),
		}
	}
	// Retorna um dominio.Itinerario com os passos reconstruídos.
	return dominio.Itinerario{Passos: passos}
}

// reservaParaResposta converte um dominio.Reserva para o formato do
// protocolo — usado tanto por ConfirmarReserva quanto por
// ConsultarReservas.
func reservaParaResposta(r dominio.Reserva) protocolo.ReservaResposta {
	trechos := make([]protocolo.TrechoDados, len(r.Itens))
	for i, item := range r.Itens {
		trechos[i] = protocolo.TrechoDados{
			EmpresaID: item.Trecho.EmpresaID,
			IDCarona:  item.Trecho.IDCarona,
			Indice:    item.Trecho.Indice,
			Origem:    string(item.Trecho.Origem),
			Destino:   string(item.Trecho.Destino),
		}
	}
	empresaID := ""
	if len(trechos) > 0 {
		empresaID = trechos[0].EmpresaID
	}
	return protocolo.ReservaResposta{
		ID:         r.Id,
		Passageiro: r.Passageiro,
		EmpresaID:  empresaID,
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

func tratarBuscarItinerarios(
	repo *estado.Repositorio,
	req protocolo.Requisicao,
	busca *consorcio.BuscaDistribuida,
) protocolo.Resposta {
	var dados protocolo.BuscarItinerariosDados

	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	// Sem serviço distribuído configurado, consulta somente a empresa local.
	if busca == nil {
		itinerarios, err := repo.BuscarItinerarios(
			dominio.Cidade(dados.Origem),
			dominio.Cidade(dados.Destino),
		)
		if err != nil {
			return erro(req, err.Error())
		}

		var resposta protocolo.BuscarItinerariosResposta
		for _, it := range itinerarios {
			resposta.Itinerarios = append(
				resposta.Itinerarios,
				itinerarioParaDados(it),
			)
		}

		return ok(req, resposta)
	}

	// A busca distribuída precisa dos parâmetros de viagem.
	data, err := time.Parse("2006-01-02", dados.Data)
	if err != nil {
		return erro(req, "data invalida: use o formato AAAA-MM-DD")
	}

	if dados.NumPassageiros <= 0 {
		return erro(req, "a quantidade de passageiros deve ser positiva")
	}

	resultado, err := busca.Buscar(
		context.Background(),
		dominio.Cidade(dados.Origem),
		dominio.Cidade(dados.Destino),
		data,
		dados.NumPassageiros,
	)
	if err != nil {
		return erro(req, err.Error())
	}

	var resposta protocolo.BuscarItinerariosResposta
	for _, it := range resultado.Itinerarios {
		resposta.Itinerarios = append(
			resposta.Itinerarios,
			itinerarioParaDados(it),
		)
	}

	resposta.BuscaCompleta = resultado.BuscaCompleta
	resposta.EmpresasIndisponiveis = resultado.EmpresasIndisponiveis

	return ok(req, resposta)
}

// tratarConfirmarReserva agora usa o usuario da sessão
func tratarConfirmarReserva(
	repo *estado.Repositorio,
	usuario string,
	req protocolo.Requisicao,
	busca *consorcio.BuscaDistribuida,
) protocolo.Resposta {
	var dados protocolo.ConfirmarReservaDados

	// 1. Desserializa os dados da requisição (trechos do itinerário escolhido)
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	// 2. Converte os trechos de []TrechoDados para dominio.Itinerario
	itinerario := dadosParaItinerario(dados.Trechos)

	// Quando a busca distribuída está configurada, a empresa à qual o
	// passageiro está conectado coordena esta transação específica.
	// Sem peers configurados, preservamos o fluxo local legado.
	if busca != nil && busca.Coordenador != nil {
		if strings.TrimSpace(req.IDRequisicao) == "" {
			return erro(req, "id_requisicao obrigatorio para reserva distribuida")
		}
		resultado, err := busca.Coordenador.ReservarComChave(
			context.Background(),
			usuario,
			itinerario,
			req.IDRequisicao,
		)
		if err != nil {
			return erro(req, err.Error())
		}
		return ok(req, resultado)
	}

	reserva, err := repo.ConfirmarReserva(usuario, itinerario)
	if err != nil {
		return erro(req, err.Error())
	}
	return ok(req, reservaParaResposta(reserva))
}

// tratarConsultarReservas agora busca reservas atreladas à sessão
func tratarConsultarReservas(
	repo *estado.Repositorio,
	usuario string,
	req protocolo.Requisicao,
	busca *consorcio.BuscaDistribuida,
) protocolo.Resposta {
	reservas, err := repo.ConsultarReservas(usuario)
	if err != nil {
		return erro(req, err.Error())
	}

	var resposta protocolo.ConsultarReservasResposta
	indices := make(map[string]int)
	chaveReserva := func(empresaID string, id int) string {
		return fmt.Sprintf("%s/%d", empresaID, id)
	}
	for _, reserva := range reservas {
		item := reservaParaResposta(reserva)
		if item.EmpresaID == "" {
			item.EmpresaID = repo.EmpresaID()
		}
		indices[chaveReserva(item.EmpresaID, item.ID)] = len(resposta.Reservas)
		resposta.Reservas = append(resposta.Reservas, item)
	}

	// O coordenador mantém o log com os IDs locais das reservas de todos os
	// participantes. Isso permite mostrar a viagem completa ao passageiro
	// conectado à empresa que iniciou a coordenação.
	if busca != nil && busca.Coordenador != nil {
		for _, tx := range repo.ListarTransacoesCoordenadasDoPassageiro(usuario) {
			for _, participante := range tx.Participantes {
				item := protocolo.ReservaResposta{
					ID:          participante.ReservaID,
					Passageiro:  tx.Passageiro,
					EmpresaID:   participante.EmpresaID,
					TransacaoID: tx.ID,
					Trechos:     itinerarioParaDados(participante.Itinerario).Trechos,
				}
				chave := chaveReserva(item.EmpresaID, item.ID)
				if indice, existe := indices[chave]; existe {
					// Substitui a representacao local pela versao enriquecida com
					// o ID da transacao global.
					resposta.Reservas[indice] = item
					continue
				}
				indices[chave] = len(resposta.Reservas)
				resposta.Reservas = append(resposta.Reservas, item)
			}
		}
	}
	return ok(req, resposta)
}

func tratarCancelarReserva(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.CancelarReservaDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	if dados.EmpresaID != "" && dados.EmpresaID != repo.EmpresaID() {
		return erro(req, "esta reserva pertence a outra empresa; conecte-se ao servidor coordenador")
	}
	if dados.TransacaoID != "" {
		return erro(req, "cancelamento de itinerario distribuido ainda nao esta disponivel; nenhuma parte foi cancelada")
	}
	if err := repo.CancelarReserva(dados.IDReserva, usuario); err != nil {
		return erro(req, err.Error())
	}

	return ok(req, map[string]string{"mensagem": "reserva cancelada"})
}

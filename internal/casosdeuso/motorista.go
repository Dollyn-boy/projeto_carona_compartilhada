package casosdeuso

import (
	"encoding/json"
	"time"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// tratarPublicarCarona: desserializa PublicarCaronaDados, converte os
// tipos do protocolo (string, []string) para os tipos de dominio
// (dominio.Cidade, time.Time), chama repo.PublicarCarona, e devolve a
// carona criada.
func tratarPublicarCarona(repo *estado.Repositorio, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.PublicarCaronaDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	rota := make([]dominio.Cidade, len(dados.Rota))
	for i, cidade := range dados.Rota {
		rota[i] = dominio.Cidade(cidade)
	}

	data, err := time.Parse("2006-01-02", dados.Data)
	if err != nil {
		return erro(req, "data invalida (use o formato AAAA-MM-DD): "+err.Error())
	}

	carona, err := repo.PublicarCarona(dados.Motorista, rota, dados.Capacidade, dados.Preco, data)
	if err != nil {
		return erro(req, err.Error())
	}

	return ok(req, caronaParaResposta(carona))
}

// tratarConsultarCaronas: leitura simples, so passa o filtro de
// motorista adiante e converte o resultado.
func tratarConsultarCaronas(repo *estado.Repositorio, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.ConsultarCaronasDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	caronas, err := repo.ConsultarCaronas(dados.Motorista)
	if err != nil {
		return erro(req, err.Error())
	}

	var resposta protocolo.ConsultarCaronasResposta
	for _, c := range caronas {
		resposta.Caronas = append(resposta.Caronas, caronaParaResposta(c))
	}

	return ok(req, resposta)
}

// tratarCancelarCarona: repassa o ID para repo.CancelarCarona; o erro
// (ex: "carona nao encontrada") já vem pronto de internal/estado.
func tratarCancelarCarona(repo *estado.Repositorio, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.CancelarCaronaDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	if err := repo.CancelarCarona(dados.IDCarona); err != nil {
		return erro(req, err.Error())
	}

	return ok(req, map[string]string{"mensagem": "carona cancelada"})
}

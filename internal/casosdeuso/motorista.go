package casosdeuso

import (
	"encoding/json"
	"time"

	"vaijunto/internal/dominio"
	"vaijunto/internal/estado"
	"vaijunto/internal/protocolo"
)

// tratarPublicarCarona agora recebe usuario da Sessão
func tratarPublicarCarona(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
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

	// SEGURANÇA: Usamos o usuario da sessão em vez de dados.Motorista!
	carona, err := repo.PublicarCarona(usuario, rota, dados.Capacidade, dados.Preco, data)
	if err != nil {
		return erro(req, err.Error())
	}

	return ok(req, caronaParaResposta(carona))
}

// tratarConsultarCaronas agora busca apenas as caronas do usuário logado
func tratarConsultarCaronas(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
	// Nota: Como não precisamos mais do dados.Motorista vindo do cliente,
	// você pode até ignorar o payload do cliente aqui se quiser.

	// SEGURANÇA: Buscamos as caronas usando o ID da sessão
	caronas, err := repo.ConsultarCaronas(usuario)
	if err != nil {
		return erro(req, err.Error())
	}

	var resposta protocolo.ConsultarCaronasResposta
	for _, c := range caronas {
		resposta.Caronas = append(resposta.Caronas, protocolo.CaronaComOcupacaoResposta{
			CaronaResposta:       caronaParaResposta(c.Carona),
			PassageirosPorTrecho: c.PassageirosPorTrecho,
		})
	}

	return ok(req, resposta)
}

// tratarCancelarCarona repassa o usuario para validar autorização
func tratarCancelarCarona(repo *estado.Repositorio, usuario string, req protocolo.Requisicao) protocolo.Resposta {
	var dados protocolo.CancelarCaronaDados
	if err := json.Unmarshal(req.Dados, &dados); err != nil {
		return erro(req, "dados invalidos: "+err.Error())
	}

	// IDEALMENTE: Atualize repo.CancelarCarona para receber (dados.IDCarona, usuarioID)
	// para garantir que o usuário não cancele a carona de outra pessoa!
	if err := repo.CancelarCarona(dados.IDCarona, usuario); err != nil {
		return erro(req, err.Error())
	}

	return ok(req, map[string]string{"mensagem": "carona cancelada"})
}

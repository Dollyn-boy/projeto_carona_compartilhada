package casosdeuso

import (
	"encoding/json"
	"vaijunto/internal/protocolo"
)

// TODO — handlers das operações do motorista. Cada um chama o pacote
// estado (nunca dominio ou concorrencia diretamente — ver README):
//   - PublicarCarona
//   - ConsultarCaronas
//   - CancelarCarona
func tratarPublicarCarona(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal(req.Dados)
	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
	}
}

func tratarConsultarCaronas(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal(req.Dados)
	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
	}
}

func tratarCancelarCarona(req protocolo.Requisicao) protocolo.Resposta {
	dados, _ := json.Marshal(req.Dados)
	return protocolo.Resposta{
		Status:       "ok",
		IDRequisicao: req.IDRequisicao,
		Dados:        dados,
	}
}

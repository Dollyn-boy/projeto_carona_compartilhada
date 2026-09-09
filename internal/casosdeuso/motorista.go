package casosdeuso

import "vaijunto/internal/protocolo"

// TODO — handlers das operações do motorista. Cada um chama o pacote
// estado (nunca dominio ou concorrencia diretamente — ver README):
//   - PublicarCarona
//   - ConsultarCaronas
//   - CancelarCarona
func tratarPublicarCarona(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}

func tratarConsultarCaronas(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}

func tratarCancelarCarona(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}

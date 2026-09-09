package casosdeuso

import "vaijunto/internal/protocolo"

// TODO — handlers das operações do passageiro. Cada um chama o pacote
// estado (nunca dominio ou concorrencia diretamente — ver README):
//   - BuscarItinerarios
//   - ConfirmarReserva
//   - ConsultarReservas
//   - CancelarReserva

func tratarConfirmarReserva(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}

func tratarConsultarReservas(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}

func tratarCancelarReserva(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}
func tratarBuscarItinerarios(req protocolo.Requisicao) protocolo.Resposta {
	panic("unimplemented")
}

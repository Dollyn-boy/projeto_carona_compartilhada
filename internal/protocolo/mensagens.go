package protocolo

import "encoding/json"

type Requisicao struct {
	Tipo         string          `json:"tipo"`
	IDRequisicao string          `json:"id_requisicao"`
	Dados        json.RawMessage `json:"dados"`
}

type Resposta struct {
	Status       string          `json:"status"`
	IDRequisicao string          `json:"id_requisicao"`
	Dados        json.RawMessage `json:"dados,omitempty"`
	Motivo       string          `json:"motivo,omitempty"`
}

type LoginDados struct {
	Usuario string `json:"email"`
	Senha   string `json:"senha"`
}

type CadastroDados struct {
	Usuario string `json:"email"`
	Senha   string `json:"senha"`
}

type PublicarCaronaDados struct {
	Motorista  string   `json:"motorista"`
	Rota       []string `json:"rota"`
	Capacidade int      `json:"capacidade"`
	Preco      int      `json:"preco"`
	Data       string   `json:"data"`
}

type ConsultarCaronasDados struct {
	Motorista string `json:"motorista"`
}

type CancelarCaronaDados struct {
	IDCarona int `json:"id_carona"`
}

type BuscarItinerariosDados struct {
	Origem         string `json:"origem"`
	Destino        string `json:"destino"`
	Data           string `json:"data"`
	NumPassageiros int    `json:"num_passageiros"`
}

type ConfirmarReservaDados struct {
	IDCarona   int    `json:"id_carona"`
	Passageiro string `json:"passageiro"`
}

type ConsultarReservasDados struct {
	Passageiro string `json:"passageiro"`
}

type CancelarReservaDados struct {
	IDReserva int `json:"id_reserva"`
}

// Defina um tipo Go por operacao (ou um campo "dados" generico via
// json.RawMessage/map, sua escolha), cobrindo pelo menos:
//   login, publicar_carona, consultar_caronas, cancelar_carona,
//   buscar_itinerarios, confirmar_reserva, consultar_reservas, cancelar_reserva
//

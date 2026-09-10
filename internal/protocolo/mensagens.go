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

// CORRECAO: a versao original deste tipo so tinha IDCarona+Passageiro —
// isso so consegue representar UM trecho de UMA carona. Mas um
// itinerario pode combinar trechos de caronas DIFERENTES (é o cenario
// central do enunciado: Salvador -> Feira -> Vitoria da Conquista com
// dois motoristas). Por isso trocamos para carregar a lista completa de
// trechos do itinerario escolhido (o mesmo formato que
// BuscarItinerariosResposta devolve em ItinerarioDados.Trechos — o
// cliente so reenvia o que recebeu).
type ConfirmarReservaDados struct {
	Passageiro string        `json:"passageiro"`
	Trechos    []TrechoDados `json:"trechos"`
}

type ConsultarReservasDados struct {
	Passageiro string `json:"passageiro"`
}

type CancelarReservaDados struct {
	IDReserva int `json:"id_reserva"`
}

// ============================================================================
// Tipos de RESPOSTA (o que o servidor devolve dentro de Resposta.Dados)
// ============================================================================

// TrechoDados e a representacao, sobre o protocolo, de um dominio.Trecho
// — usado tanto para devolver itinerarios encontrados (dentro de
// ItinerarioDados) quanto para o cliente reenviar o itinerario escolhido
// em ConfirmarReservaDados.
type TrechoDados struct {
	IDCarona string `json:"id_carona"`
	Indice   int    `json:"indice"`
	Origem   string `json:"origem"`
	Destino  string `json:"destino"`
}

// ItinerarioDados e um dominio.Itinerario representado sobre o protocolo:
// so a sequencia ordenada de trechos que o compoe.
type ItinerarioDados struct {
	Trechos []TrechoDados `json:"trechos"`
}

// BuscarItinerariosResposta e o Dados devolvido por "buscar_itinerarios":
// a lista de itinerarios possiveis encontrados (pode ter mais de um,
// pois internal/estado usa dominio.BuscarTodosItinerarios).
type BuscarItinerariosResposta struct {
	Itinerarios []ItinerarioDados `json:"itinerarios"`
}

// CaronaResposta e um dominio.Carona representado sobre o protocolo,
// devolvido por "publicar_carona" e dentro de ConsultarCaronasResposta.
type CaronaResposta struct {
	ID         int      `json:"id"`
	Motorista  string   `json:"motorista"`
	Rota       []string `json:"rota"`
	Capacidade int      `json:"capacidade"`
	Preco      int      `json:"preco"`
	Data       string   `json:"data"`
}

// ConsultarCaronasResposta e o Dados devolvido por "consultar_caronas".
type ConsultarCaronasResposta struct {
	Caronas []CaronaResposta `json:"caronas"`
}

// ReservaResposta e um dominio.Reserva representado sobre o protocolo,
// devolvido por "confirmar_reserva" e dentro de ConsultarReservasResposta.
type ReservaResposta struct {
	ID         int           `json:"id"`
	Passageiro string        `json:"passageiro"`
	Trechos    []TrechoDados `json:"trechos"`
}

// ConsultarReservasResposta e o Dados devolvido por "consultar_reservas".
type ConsultarReservasResposta struct {
	Reservas []ReservaResposta `json:"reservas"`
}

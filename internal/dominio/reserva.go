package dominio

// Reserva representa a confirmacao de um itinerario, composto por um ou mais trechos.
type Reserva struct {
	Id         int           `json:"id"`
	Passageiro string        `json:"passageiro"`
	Itens      []ItemReserva `json:"itens"`
}

type ItemReserva struct {
	CaronaID int    `json:"carona_id"`
	Trecho   Trecho `json:"trecho"`
}

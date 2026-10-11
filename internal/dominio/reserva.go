package dominio

type Reserva struct {
	Id         int
	Passageiro string
	Itens      []ItemReserva
}

type ItemReserva struct {
	CaronaID int
	Trecho   Trecho
}

// Package dominio define os dados centrais e as regras de negocio puras do VaiJunto.
package dominio

import "time"

// Carona descreve uma oferta publicada por um motorista.
type Carona struct {
	Id         int       `json:"id"`
	Motorista  string    `json:"motorista"`
	Rota       []Cidade  `json:"rota"`
	Preco      int       `json:"preco"`
	Capacidade int       `json:"capacidade"`
	Data       time.Time `json:"data"`
}

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

// LoginResposta e o Dados devolvido por um login bem-sucedido — o Role
// deixa o cliente saber qual menu mostrar sem precisar perguntar de
// novo a cada login.
type LoginResposta struct {
	Mensagem string `json:"mensagem"`
	Role     string `json:"role"`
}

type CadastroDados struct {
	Usuario string `json:"email"`
	Role    string `json:"role"`
	Senha   string `json:"senha"`
}

// ============================================================================
// Tipos de REQUISIÇÃO (o que o cliente envia)
// SEGURANÇA: Os campos "Motorista" e "Passageiro" foram removidos daqui.
// A identidade agora é extraída da Sessão no pacote casosdeuso!
// ============================================================================

type PublicarCaronaDados struct {
	Rota       []string `json:"rota"`
	Capacidade int      `json:"capacidade"`
	Preco      int      `json:"preco"`
	Data       string   `json:"data"`
}

// Struct vazio, pois o servidor já sabe quem é o motorista pela sessão.
// Se o cliente não mandar campo "dados" no JSON, você pode até pular o Unmarshal.
type ConsultarCaronasDados struct{}

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
	Trechos []TrechoDados `json:"trechos"`
}

// Struct vazio, pois o servidor já sabe quem é o passageiro pela sessão.
type ConsultarReservasDados struct{}

type CancelarReservaDados struct {
	IDReserva int `json:"id_reserva"`
}

// ============================================================================
// Tipos de RESPOSTA (o que o servidor devolve dentro de Resposta.Dados)
// ============================================================================

type TrechoDados struct {
	IDCarona string `json:"id_carona"`
	Indice   int    `json:"indice"`
	Origem   string `json:"origem"`
	Destino  string `json:"destino"`
}

type ItinerarioDados struct {
	Trechos []TrechoDados `json:"trechos"`
}

type BuscarItinerariosResposta struct {
	Itinerarios []ItinerarioDados `json:"itinerarios"`
}

type CaronaResposta struct {
	ID         int      `json:"id"`
	Motorista  string   `json:"motorista"` // Mantido para o cliente saber de quem é a carona
	Rota       []string `json:"rota"`
	Capacidade int      `json:"capacidade"`
	Preco      int      `json:"preco"`
	Data       string   `json:"data"`
}

type ConsultarCaronasResposta struct {
	Caronas []CaronaResposta `json:"caronas"`
}

type ReservaResposta struct {
	ID         int           `json:"id"`
	Passageiro string        `json:"passageiro"`
	Trechos    []TrechoDados `json:"trechos"`
}

type ConsultarReservasResposta struct {
	Reservas []ReservaResposta `json:"reservas"`
}

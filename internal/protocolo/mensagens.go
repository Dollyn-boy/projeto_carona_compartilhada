
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
    Email string `json:"email"`
    Senha string `json:"senha"`
}


// Defina um tipo Go por operacao (ou um campo "dados" generico via
// json.RawMessage/map, sua escolha), cobrindo pelo menos:
//   login, publicar_carona, consultar_caronas, cancelar_carona,
//   buscar_itinerarios, confirmar_reserva, consultar_reservas, cancelar_reserva
//
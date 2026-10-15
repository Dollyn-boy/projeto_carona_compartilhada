package dominio

// Cidade e o nome textual de uma cidade usada na rota.
type Cidade string

// Trecho representa o intervalo entre duas cidades adjacentes de uma rota.
type Trecho struct {
	IDCarona string `json:"id_carona"`
	Indice   int    `json:"indice"`
	Origem   Cidade `json:"origem"`
	Destino  Cidade `json:"destino"`
}

// TrechosDaRota transforma uma rota ordenada em seus trechos elementares.
// A rota [A, B, C] produz A-B (indice 0) e B-C (indice 1).
func TrechosDaRota(idCarona string, rota []Cidade) []Trecho {
	if len(rota) < 2 {
		return []Trecho{}
	}
	trechos := make([]Trecho, 0, len(rota)-1)
	for i := 0; i < len(rota)-1; i++ {
		trechos = append(trechos, Trecho{
			IDCarona: idCarona,
			Indice:   i,
			Origem:   rota[i],
			Destino:  rota[i+1],
		})
	}
	return trechos
}

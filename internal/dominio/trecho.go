package dominio

// Cidade e o nome textual de uma cidade usada na rota.
type Cidade string

// Trecho representa o intervalo entre duas cidades adjacentes
// de uma carona pertencente a uma empresa.
type Trecho struct {
	EmpresaID string `json:"empresa_id"`
	IDCarona  string `json:"id_carona"`
	Indice    int    `json:"indice"`
	Origem    Cidade `json:"origem"`
	Destino   Cidade `json:"destino"`
}

// TrechosDaRota transforma uma rota ordenada em trechos elementares.
//
// Exemplo:
// TrechosDaRota("empresa-a", "1", [A, B, C])
// produz:
// empresa-a / carona 1 / trecho 0: A-B
// empresa-a / carona 1 / trecho 1: B-C
func TrechosDaRota(
	empresaID string,
	idCarona string,
	rota []Cidade,
) []Trecho {
	if len(rota) < 2 {
		return []Trecho{}
	}

	trechos := make([]Trecho, 0, len(rota)-1)

	for i := 0; i < len(rota)-1; i++ {
		trechos = append(trechos, Trecho{
			EmpresaID: empresaID,
			IDCarona:  idCarona,
			Indice:    i,
			Origem:    rota[i],
			Destino:   rota[i+1],
		})
	}

	return trechos
}

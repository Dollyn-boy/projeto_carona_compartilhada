package protocolo

import (
	"bufio"
	"encoding/json"
	"net"
)

// EscreverMensagem serializa a mensagem msg em JSON e a envia pelo socket TCP conn, seguida de um caractere de nova linha.
func EscreverMensagem(conn net.Conn, msg any) error {
	dados, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	dados = append(dados, '\n')

	// Escreve os bytes da mensagem JSON no socket TCP, seguido de um caractere de nova linha.
	_, err = conn.Write(dados)

	return err
}

func LerMensagem(reader *bufio.Reader, msg any) error {
	dados, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}

	return json.Unmarshal(dados, msg)
}

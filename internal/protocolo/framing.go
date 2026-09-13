package protocolo

import (
	"bufio"
	"encoding/json"
	"net"
)

func EscreverMensagem(conn net.Conn, msg any) error {
	dados, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	dados = append(dados, '\n')

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

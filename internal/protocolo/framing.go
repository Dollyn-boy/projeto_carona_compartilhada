package protocolo

// TODO — este arquivo deve conter as funcoes de leitura/escrita de
// mensagens sobre uma conexao TCP (framing), usadas tanto pelo servidor
// quanto pelos clientes.
//
// TCP e um fluxo de bytes sem fronteiras de mensagem — escolha UMA
// estrategia de framing e aplique nos dois lados:
//   a) delimitador: uma mensagem JSON por linha, terminada em '\n'
//      (simples, funciona bem porque JSON minificado nao contem '\n' cru)
//   b) prefixo de tamanho: 4 bytes de tamanho + payload
//      (mais robusto para payloads binarios/arbitrarios)
//
// Funcoes sugeridas:
//   EscreverMensagem(conn net.Conn, msg any) error
//   LerMensagem(conn net.Conn) (Mensagem, error)

// Package servidor contem a camada de rede do lado do servidor: aceitar
// conexoes, fazer o parsing das mensagens e rotear para os casos de uso.
package servidor

// TODO — este arquivo deve conter:
//   1. A funcao que sobe o net.Listen("tcp", ":PORTA") e o loop de Accept().
//   2. Uma goroutine por conexao aceita (mesmo padrao do exercico de
//      eco) — a queda de um cliente nunca pode afetar os demais.
//   3. Dentro de cada goroutine: um loop lendo mensagens (via
//      internal/protocolo), identificando o "tipo" e chamando o
//      handler correspondente (ver handlers_motorista.go e
//      handlers_passageiro.go).
//   4. Onde a sessao/autenticacao da conexao e guardada — considere uma
//      struct por conexao que acumula o usuario logado.

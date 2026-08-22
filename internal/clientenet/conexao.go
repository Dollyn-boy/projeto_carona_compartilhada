// Package clientenet e a camada de rede compartilhada pelos dois
// clientes (motorista e passageiro): conectar, enviar e receber
// mensagens do protocolo.
package clientenet

// TODO — este arquivo deve conter:
//   - uma funcao Conectar(endereco string) que faz net.Dial("tcp", ...)
//   - funcoes para enviar uma requisicao e aguardar a resposta
//     correspondente, usando internal/protocolo para serializar e
//     fazer o framing
//
// Este pacote nao deve conter nenhuma logica de interface — so rede. A
// interface de terminal (menus, prompts) fica em cmd/cliente-motorista
// e cmd/cliente-passageiro, chamando este pacote.

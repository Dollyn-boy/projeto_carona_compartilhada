package main

// Ponto de entrada do servidor central do VaiJunto.
//
// TODO:
//   1. Ler configuracao (porta de escuta, etc.) — flag ou variavel de ambiente.
//   2. Inicializar o estado do dominio (ver internal/dominio).
//   3. Inicializar o controle de concorrencia sobre esse estado (ver internal/concorrencia).
//   4. Subir o listener TCP e o loop de aceitacao de conexoes
//      (ver internal/servidor/dispatcher.go).
//   5. Tratar encerramento gracioso do servidor (sinal SIGINT/SIGTERM), se desejar.
//
// Lembrete do enunciado: servidor unico, sem replicas — a queda de um
// cliente nunca pode interromper o servico nem corromper o estado.

func main() {
	// TODO: implementar o bootstrap do servidor.
}

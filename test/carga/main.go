// Harness de teste automatizado de concorrencia: sobe multiplos
// clientes simultaneos disputando os mesmos trechos contra um servidor
// ja em execucao, e valida as invariantes exigidas pelo enunciado.
package main

// TODO:
//   1. Configurar o endereco do servidor alvo (flag ou constante).
//   2. Disparar N goroutines, cada uma abrindo sua propria conexao (via
//      internal/clientenet) e tentando confirmar itinerarios que se
//      sobrepoem nos mesmos trechos — use sync.WaitGroup para esperar
//      todas terminarem.
//   3. Registrar o resultado de cada tentativa (sucesso/falha) e o
//      tempo de resposta (time.Since) de cada uma.
//   4. Ao final, validar as invariantes:
//        - a soma de assentos confirmados por trecho nunca excede a
//          capacidade;
//        - nenhuma reserva aparece parcialmente confirmada.
//   5. Imprimir um resumo com tempo medio/percentis de resposta sob carga.
//
// Dica: rode o servidor separadamente com go test -race ativo durante
// esse teste para pegar qualquer condicao de corrida residual.

func main() {
	// TODO: implementar o harness de carga.
}

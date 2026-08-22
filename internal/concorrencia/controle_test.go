package concorrencia

// TODO — testes concorrentes:
//   - disparar varias goroutines tentando reservar os MESMOS trechos ao
//     mesmo tempo, e validar que a soma de assentos confirmados nunca
//     excede a capacidade;
//   - validar que nenhuma reserva com multiplos trechos fica
//     parcialmente confirmada;
//   - validar que "quem confirma primeiro" mantem a preferencia.
//
// Rode sempre com: go test -race ./internal/concorrencia/...

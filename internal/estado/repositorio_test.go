package estado

// TODO — testes de integracao desta camada (dominio + concorrencia ja
// combinados através do Repositorio):
//   - publicar carona e confirmar reservas concorrentes disputando os
//     mesmos trechos (varias goroutines chamando ConfirmarReserva ao
//     mesmo tempo);
//   - validar que a soma de assentos confirmados nunca excede a
//     capacidade e que nenhuma reserva fica parcial;
//   - rodar sempre com: go test -race ./internal/estado/...
//
// Isso complementa (nao substitui) os testes mais focados de
// internal/dominio e internal/concorrencia — aqui voce testa as tres
// camadas trabalhando juntas.

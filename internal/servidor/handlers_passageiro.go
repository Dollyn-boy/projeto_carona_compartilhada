package servidor

// TODO — este arquivo deve conter os handlers das operacoes do
// passageiro, cada um traduzindo uma mensagem de protocolo em chamadas
// aos pacotes dominio e concorrencia:
//   - BuscarItinerarios: usa internal/dominio (busca.go) para devolver
//     os itinerarios possiveis entre origem, destino e data.
//   - ConfirmarReserva: recebe o itinerario completo escolhido e chama
//     o pacote concorrencia para confirmar atomicamente todos os
//     trechos envolvidos, ou nenhum.
//   - ConsultarReservas / CancelarReserva: consultam ou liberam os
//     trechos de uma reserva do passageiro autenticado na conexao.

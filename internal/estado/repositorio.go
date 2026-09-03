// Package estado e a camada "Estado em memoria" da arquitetura — o
// unico pacote que combina os tipos de internal/dominio com as travas
// de internal/concorrencia para expor operacoes SEGURAS (ja protegidas
// contra corrida) para o pacote casosdeuso chamar.
//
// Nenhum outro pacote deve tocar diretamente nas estruturas de dados
// guardadas aqui — e essa fronteira que garante que toda leitura ou
// escrita passa pelo controle de concorrencia, sem excecao.
package estado

// TODO — este arquivo deve conter:
//   - a struct que guarda o estado de fato (ex: mapa de caronas por ID,
//     contadores de assento por trecho, mapa de reservas por passageiro);
//   - um construtor, ex: NovoRepositorio() *Repositorio;
//   - metodos que expoem operacoes seguras usando internal/concorrencia
//     por baixo e internal/dominio para validacoes/tipos, como:
//       (r *Repositorio) PublicarCarona(...) (dominio.Carona, error)
//       (r *Repositorio) BuscarItinerarios(...) ([]Itinerario, error)   — leitura, nao trava nada
//       (r *Repositorio) ConfirmarReserva(...) (dominio.Reserva, error) — escrita atomica sobre 1+ trechos
//       (r *Repositorio) CancelarCarona(...) error
//       (r *Repositorio) CancelarReserva(...) error
//
// Pense nesta struct como um repositorio em memoria, nao como um banco
// de dados — nao ha nada aqui alem de estruturas Go protegidas por
// travas (ver a conversa sobre por que nao usar um SGBD neste projeto).

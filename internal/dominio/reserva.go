package dominio

// TODO — este arquivo deve conter o tipo Reserva.
//
// Uma reserva pertence a um passageiro e e composta por um ou mais
// itens, onde cada item referencia:
//   - a carona de origem;
//   - o trecho ou intervalo de trechos reservados dentro daquela carona
//     (ex: da cidade B ate a cidade D).
//
// Um itinerario pode combinar trechos de caronas DIFERENTES — nesse caso
// a reserva tem multiplos itens apontando para caronas distintas.
//
// Regra de ouro: a confirmacao de uma reserva com multiplos itens deve
// ser atomica — ou todos os itens sao confirmados, ou nenhum e.

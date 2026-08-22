package dominio

// TODO — testes unitarios da logica de disponibilidade por trecho:
//   - publicar uma carona e verificar os contadores iniciais;
//   - reservar um intervalo de trechos e verificar que os contadores
//     corretos (e somente eles) foram decrementados;
//   - tentar reservar mais assentos do que o disponivel deve falhar;
//   - buscar itinerarios que exigem combinar trechos de caronas
//     diferentes.
//
// Estes testes NAO precisam de concorrencia ainda — isso fica a cargo
// de internal/concorrencia/controle_test.go.

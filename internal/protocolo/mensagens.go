// Package protocolo define o contrato de mensagens trocadas entre
// cliente e servidor — o formato deve estar espelhado em docs/protocolo.md.
package protocolo

// TODO — este arquivo deve conter os tipos das mensagens do protocolo.
//
// Sugestao de envelope comum para toda requisicao e resposta:
//   Requisicao: { "tipo": "...", "id_requisicao": "...", "dados": {...} }
//   Resposta:   { "status": "ok" | "erro", "id_requisicao": "...", "dados" | "motivo": ... }
//
// Defina um tipo Go por operacao (ou um campo "dados" generico via
// json.RawMessage/map, sua escolha), cobrindo pelo menos:
//   login, publicar_carona, consultar_caronas, cancelar_carona,
//   buscar_itinerarios, confirmar_reserva, consultar_reservas, cancelar_reserva
//
// Lembrete do enunciado: o receptor deve validar e descartar mensagens
// malformadas — pense em como sinalizar isso na resposta de erro.

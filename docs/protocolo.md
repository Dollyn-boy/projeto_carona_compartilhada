# Protocolo de aplicação — VaiJunto

> Especificação completa do protocolo entre cliente e servidor, conforme
> exigido pelo enunciado. Preencha cada seção conforme for definindo e
> implementando o protocolo (ver internal/protocolo).

## 1. Envelope da mensagem

[Descreva aqui o formato comum de toda requisição e resposta: campos,
tipos, e como o receptor identifica e valida uma mensagem malformada.]

## 2. Framing sobre TCP

[Descreva a estratégia escolhida: delimitador ou prefixo de tamanho, e
por quê.]

## 3. Fluxo de conexão e desconexão

[Descreva o que acontece quando um cliente conecta (ex: precisa
autenticar antes de qualquer outra operação?) e o que acontece quando a
conexão cai no meio de uma operação.]

## 4. Operações

### login
[campos da requisição, campos da resposta, exemplo de mensagem]

### publicar_carona
[...]

### consultar_caronas
[...]

### cancelar_carona
[...]

### buscar_itinerarios
[...]

### confirmar_reserva
[...]

### consultar_reservas
[...]

### cancelar_reserva
[...]

## 5. Códigos de erro

[Liste os motivos de erro possíveis e como são representados na resposta.]

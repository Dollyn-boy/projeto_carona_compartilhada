
# API REST de Comunicação entre Empresas

## Atenção

A arquitetura do projeto tem dois níveis de comunicação:
- Cliente → servidor: o cliente envia mensagens TCP, como buscar_itinerarios.
- Servidor → servidor: o serviço distribuído consulta /v1/trechos por HTTP.

## 1. Objetivo

A API REST permite que uma empresa do VaiJunto disponibilize
informações sobre seus trechos de caronas para outras empresas
participantes do consórcio.

Cada empresa executa sua própria instância do servidor, possui
seu identificador e mantém seu próprio repositório de dados.

A API REST é usada na comunicação entre servidores. A comunicação
entre o cliente do VaiJunto e o servidor ao qual ele se conecta
continua utilizando o protocolo TCP definido no projeto.

## 2. Endereçamento

Cada servidor possui duas interfaces de rede:

- TCP: recebe requisições dos clientes VaiJunto.
- REST: disponibiliza endpoints para comunicação entre empresas.

Configuração de referência:

| Serviço | Porta TCP no container | Porta REST no container |
|---|---:|---:|
| empresa-a | 8080 | 8085 |
| empresa-b | 8080 | 8085 |
| empresa-c | 8080 | 8085 |

As portas podem ser iguais nos diferentes containers porque cada
container possui sua própria rede. Na rede Docker, os servidores
são identificados pelos nomes dos serviços.

Exemplo: http://servidor-b:8085/v1/health

## 3. Identificação da empresa

Cada servidor é configurado por meio da variável de ambiente:

VAIJUNTO_EMPRESA_ID

O identificador deve ser estável e único dentro do consórcio.

## 4. Endpoint de saúde

### Requisição

GET /v1/health

### Resposta de sucesso — HTTP 200

{
  "status": "ok",
  "empresa_id": "empresa-a"
}

O campo `status` indica que o endpoint está respondendo.
O campo `empresa_id` identifica a empresa que respondeu.

## 5. Consulta de trechos

### Requisição

GET /v1/trechos?data=AAAA-MM-DD&passageiros=N

### Parâmetros

- `data`: data da viagem no formato AAAA-MM-DD.
- `passageiros`: quantidade de passageiros, como inteiro positivo.

### Resposta de sucesso — HTTP 200

{
  "empresa_id": "empresa-a",
  "trechos": [
    {
      "trecho": {
        "empresa_id": "empresa-a",
        "id_carona": "carona-123",
        "indice": 0,
        "origem": "Feira de Santana",
        "destino": "Salvador"
      },
      "assentos_disponiveis": 3,
      "preco": 25
    }
  ]
}

A lista contém os trechos locais que atendem aos critérios da
consulta. Uma lista vazia indica que não foram encontrados trechos
correspondentes, e não que houve uma falha na API.

Os campos do exemplo devem corresponder aos DTOs efetivamente
implementados no pacote `internal/protocolo`.

## 6. Tratamento de erros

| Código HTTP | Significado |
|---|---|
| 200 | Requisição concluída com sucesso. |
| 400 | Parâmetros ausentes ou inválidos. |
| 405 | Método HTTP não permitido. |
| 500 | Falha interna inesperada. |

Formato de erro:

{
  "erro": "mensagem explicativa"
}

A mensagem não deve expor senhas, caminhos internos, dados privados
ou detalhes da implementação.

## 7. Configuração dos peers

A variável de ambiente `VAIJUNTO_PEERS` pode definir empresas
remotas no formato:

VAIJUNTO_PEERS=empresa-b=http://servidor-b:8085,empresa-c=http://servidor-c:8085

Dentro da rede Docker, deve-se utilizar o nome do serviço como host,
e não `localhost`, para alcançar outro container.

## 8. Testes

Os testes automatizados devem verificar:

- resposta e identificação do endpoint de saúde;
- consulta válida de trechos;
- rejeição de parâmetros inválidos;
- rejeição de métodos não permitidos;
- tratamento de falhas do repositório;
- formato dos corpos JSON.

Os testes devem utilizar servidores HTTP de teste, sem depender
de portas externas ou de outros containers.


## 10. Operações de participação em transação

A etapa 6 adiciona quatro endpoints para que uma empresa participe de uma
transação coordenada por outra empresa. O identificador da transação é
compartilhado por todos os participantes.

Os comandos recebidos de outra empresa devem incluir o cabeçalho:

```http
X-VaiJunto-Empresa: empresa-a
```

O valor deve corresponder a um peer configurado em `VAIJUNTO_PEERS` na
empresa participante. Essa verificação é uma proteção básica para a rede
privada do Docker; não substitui autenticação criptográfica entre serviços.

### 10.1 Preparar

```http
POST /v1/transacoes/{id}/preparar
Content-Type: application/json
X-VaiJunto-Empresa: empresa-a
```

Corpo:

```json
{
  "passageiro": "ana",
  "trechos": [
    {
      "empresa_id": "empresa-b",
      "id_carona": "8",
      "indice": 0,
      "origem": "Alagoinhas",
      "destino": "Salvador"
    }
  ]
}
```

A empresa valida todos os trechos locais, desconta temporariamente os
assentos e persiste a transação como `PREPARADA`. A preparação repetida com
o mesmo ID e os mesmos dados é idempotente.

Resposta HTTP 200:

```json
{
  "transacao_id": "empresa-a-1",
  "empresa_id": "empresa-b",
  "estado": "PREPARADA"
}
```

### 10.2 Confirmar

```http
POST /v1/transacoes/{id}/confirmar
X-VaiJunto-Empresa: empresa-a
```

A empresa transforma a preparação em uma reserva local definitiva, sem
 descontar os assentos novamente. Uma repetição retorna o mesmo identificador
de reserva.

Resposta HTTP 200:

```json
{
  "transacao_id": "empresa-a-1",
  "empresa_id": "empresa-b",
  "estado": "CONFIRMADA",
  "reserva_id": 12
}
```

### 10.3 Abortar

```http
POST /v1/transacoes/{id}/abortar
X-VaiJunto-Empresa: empresa-a
```

Se a transação estiver `PREPARADA`, os assentos temporários são devolvidos e
o estado passa para `ABORTADA`. Repetir o aborto não devolve assentos uma
segunda vez. Se o participante ainda não recebeu a preparação, registra um
estado `ABORTADA` para impedir que uma preparação atrasada reabra o ID.
Uma transação `CONFIRMADA` não pode ser abortada por essa operação.

Resposta HTTP 200:

```json
{
  "transacao_id": "empresa-a-1",
  "empresa_id": "empresa-b",
  "estado": "ABORTADA"
}
```

### 10.4 Consultar estado local

```http
GET /v1/transacoes/{id}
X-VaiJunto-Empresa: empresa-a
```

Resposta HTTP 200:

```json
{
  "transacao_id": "empresa-a-1",
  "empresa_id": "empresa-b",
  "estado": "PREPARADA"
}
```

A consulta retorna o estado mantido pelo participante, não a visão global do
coordenador.

### 10.5 Códigos de resposta das operações de transação

| Código | Significado |
|---|---|
| 200 | Operação aceita ou repetição idempotente reconhecida. |
| 400 | Corpo de preparação malformado ou incompleto. |
| 403 | Empresa coordenadora ausente ou não configurada como peer. |
| 404 | Transação local inexistente em uma confirmação ou consulta. |
| 409 | Preparação recusada, falta de assento ou transição incompatível com o estado atual. |
| 503 | Repositório sem suporte às operações de transação. |

O aborto de uma transação ainda desconhecida retorna 200 porque cria o registro
`ABORTADA` (tombstone), que impede uma preparação atrasada.

## 11. Coordenação de uma reserva distribuída

A empresa à qual o passageiro está conectado coordena somente aquela
transação. Na próxima reserva, qualquer outra empresa poderá atuar como
coordenadora. Não existe um servidor central permanente.

O fluxo executado por `internal/consorcio.Coordenador` é:

1. Validar a continuidade do itinerário e particionar os trechos por `EmpresaID`.
2. Gerar um ID com prefixo da empresa coordenadora e contador persistido.
3. Gravar o log `PREPARANDO` no JSON da coordenadora.
4. Registrar a tentativa antes de chamar cada participante e enviar `preparar`.
5. Se uma preparação falhar, gravar a decisão `ABORTAR` antes de enviar aborto
   idempotente aos participantes cuja preparação foi tentada.
6. Se todos prepararem, gravar `CONFIRMAR` no log durável antes de enviar qualquer
   confirmação.
7. Enviar `confirmar`, registrar os ACKs e apresentar `CONFIRMADA` somente quando
   todos os participantes confirmarem.

O log de coordenação fica no JSON da empresa que iniciou a reserva. Cada empresa
mantém, separadamente, sua transação local e sua reserva local.

Estados visíveis no resultado TCP:

- `CONFIRMADA`: todos os participantes reconheceram a decisão de confirmação.
- `ABORTADA`: todos os participantes cuja preparação foi tentada reconheceram o aborto.
- `PENDENTE_CONFIRMAR`: a decisão durável é confirmar, mas falta ACK de uma ou
  mais empresas. Não é uma reserva confirmada para o passageiro.
- `PENDENTE_ABORTAR`: a decisão durável é abortar, mas falta ACK de uma ou mais
  empresas.

Uma decisão `CONFIRMAR` não é convertida em aborto por causa de timeout. A
coordenadora mantém a decisão e pode usar `RetomarDecisao` para reenviar o comando
idempotente aos participantes que ainda não reconheceram a decisão.

## 12. Recuperação automática

Na inicialização e periodicamente, a empresa coordenadora executa
`Coordenador.RecuperarPendentes`. O ciclo também pode ser disparado por uma
chamada explícita a `RetomarDecisao`.

A política implementada é:

- `DECISAO_PENDENTE` com decisão `CONFIRMAR`: reenviar confirmação aos
  participantes sem ACK;
- `DECISAO_PENDENTE` com decisão `ABORTAR`: reenviar aborto aos participantes
  cuja preparação foi tentada e que ainda não reconheceram o aborto;
- `PREPARANDO` sem decisão durável: registrar `ABORTAR` e distribuir o aborto.

A terceira regra é segura sob a ordem de execução adotada pelo projeto: a
coordenadora persiste `CONFIRMAR` antes de enviar qualquer confirmação. Portanto,
se o log recuperado ainda não contém decisão, nenhum participante deveria ter
recebido uma ordem válida de confirmação. Participantes cuja mensagem de
preparação estava atrasada recebem um aborto idempotente; o tombstone impede que
uma preparação posterior reabra a transação.

Falhas de entrega são retornadas pela rotina de recuperação e registradas nos
logs do servidor. O ciclo periódico tenta novamente sem trocar a decisão. Uma
resposta `PENDENTE_CONFIRMAR` ou `PENDENTE_ABORTAR` não representa conclusão.

### Hipóteses e limites

A recuperação pressupõe que o volume JSON da coordenadora foi preservado e que
a empresa coordenadora eventualmente volta a funcionar. Se ela ficar
permanentemente indisponível ou perder seu disco depois de participantes terem
preparado recursos, estes podem permanecer bloqueados. Resolver a perda
permanente do coordenador exigiria replicação do log/decisão ou outro protocolo
consensual, fora do escopo desta implementação.

A idempotência da solicitação TCP usa `IDRequisicao` como chave. O cliente grava
localmente o mapeamento entre a assinatura da seleção e essa chave em
`VAIJUNTO_CLIENTE_ESTADO` para reutilizá-la após timeout/reinicialização. A chave
só é removida quando o cliente recebe estado terminal (`CONFIRMADA` ou
`ABORTADA`). Clientes externos ao terminal devem igualmente reutilizar o mesmo
`IDRequisicao` ao repetir uma tentativa cujo resultado ficou desconhecido.

O cancelamento de um itinerário distribuído ainda não é implementado como uma
transação global. Nesta versão, o servidor bloqueia o cancelamento local de
reservas marcadas como coordenadas, evitando liberar apenas parte do itinerário.
Um fluxo global de cancelamento requer decisão e operações locais próprias.

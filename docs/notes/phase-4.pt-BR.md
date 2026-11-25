# Notas da Phase 4

[English version](phase-4.md)

Clientes repetem requisições. O load balancer desiste em 5 s enquanto o banco responde em 6 s, o celular perde a rede, uma fila entrega a mesma mensagem duas vezes. O cliente não tem como saber se a primeira requisição já tirou o estoque, e a única coisa segura que ele pode fazer é mandar de novo. Sem idempotência, cada retry numa flash sale segura unidades para uma reserva que ninguém vai pagar.

`POST /reservations` agora exige o header `Idempotency-Key`. O desenho segue o modelo da Stripe e o [`draft-ietf-httpapi-idempotency-key-header`](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/).

| Requisição | Resposta |
|---|---|
| sem chave, em branco ou com mais de 255 caracteres | `400` |
| chave nova | `201`, ou os `404` / `409` / `422` de sempre |
| chave conhecida, mesmo `sku` e `quantity` | `201`, a mesma reserva, `Idempotent-Replayed: true` |
| chave conhecida, `sku` ou `quantity` diferente | `422` |

## A chave é commitada junto com o estoque

`ReserveOnce` roda numa única transação:

1. `INSERT INTO idempotency_keys (key) ... ON CONFLICT DO NOTHING`.
2. Se a chave já existe, lê a reserva dela e a devolve, ou recusa com `422` se o corpo for diferente.
3. Se a chave é nova, roda o CTE de reserva da Fase 3 e aponta a chave para a reserva criada.

A chave e a baixa de estoque são commitadas juntas ou não são commitadas. Se a instância cai depois da reserva e antes da resposta, a chave já está gravada, e o retry recebe a reserva de volta. Se cai antes do commit, não existe reserva nem chave, e o retry começa do zero. Com a chave num store separado, como o Redis, gravada depois da reserva, sobraria um intervalo entre as duas escritas em que um retry reservaria de novo.

## O que um retry concorrente espera

Quando duas requisições inserem a mesma chave no mesmo instante, as duas batem no índice da primary key, e o Postgres faz a segunda esperar a transação da primeira terminar:

- se a primeira commita, o `ON CONFLICT DO NOTHING` da segunda não insere nada, e ela devolve a reserva;
- se a primeira faz rollback, porque o estoque acabou, por exemplo, o insert da segunda passa, e ela reserva por conta própria.

Não existe lock em Go nem `SELECT` seguido de `INSERT`. O teste manda a mesma chave 50 vezes ao mesmo tempo, por dois pools, e o resultado é uma reserva, uma baixa de estoque e 49 replays.

A chave é inserida **antes** da reserva por causa da última unidade. Suponha estoque 1 e duas requisições com a mesma chave, mas com a reserva rodando primeiro e a chave sendo gravada depois. A primeira pega a unidade. A segunda espera na linha do produto, encontra estoque 0 e responde `409`, mesmo com a reserva dela já existindo. Há um teste para esse caso.

## Onde isto é diferente da Stripe

- **Uma requisição que falhou não deixa chave.** Um `409 insufficient stock` desfaz a chave junto com todo o resto, então o retry é avaliado de novo e pode dar certo se o estoque voltar. Isso é seguro porque a primeira tentativa não tirou nada. A Stripe também faz replay de erros, porque uma requisição que cobrou dinheiro precisa mostrar ao cliente o resultado original. A propriedade de que este serviço precisa é *no máximo uma reserva por chave*, e ela vale nos dois casos.
- **O replay devolve a reserva como ela está agora.** Se o cliente confirmou nesse meio-tempo, o replay mostra `confirmed`. A Stripe guarda a resposta original e devolve byte a byte. Fazer isso aqui obrigaria a camada de banco a guardar corpos HTTP, e o estado atual é mais útil para um cliente que está repetindo a requisição.

## Chaves guardadas por 24 horas

As chaves duram `IDEMPOTENCY_KEY_TTL`, que por padrão é `24h`, a janela que a Stripe documenta. Depois disso, um segundo worker as apaga em lotes, com o mesmo padrão `FOR UPDATE SKIP LOCKED` do expirador, então todas as instâncias podem rodá-lo. Os dois workers agora logam com um campo `worker`, que diz qual job gerou cada linha.

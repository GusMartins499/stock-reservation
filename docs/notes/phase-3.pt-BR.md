# Notas da Phase 3

[English version](phase-3.md)

Uma reserva segura o estoque por um tempo limitado. O comprador confirma depois de pagar ou cancela. Se nada disso acontece antes de `expires_at`, as unidades voltam para a venda. Sem isso, todo checkout abandonado tira unidades da prateleira para sempre, e uma flash sale esgota para gente que nunca pagou.

## A máquina de estados

| De | Evento | Para | Estoque |
|---|---|---|---|
| — | reservar | `pending` | diminui |
| `pending` | confirmar antes de `expires_at` | `confirmed` | não muda |
| `pending` | cancelar | `cancelled` | volta |
| `pending` | `expires_at` passa | `expired` | volta |

Qualquer outra transição é recusada com `409`. Uma reserva confirmada fica com as unidades: `confirmed` quer dizer vendido, então o estoque já estava certo desde a reserva.

Cada transição é um único comando SQL, e o `WHERE status = 'pending'` é a proteção. Quando duas requisições disputam a mesma reserva, o Postgres trava a linha para o primeiro `UPDATE`; o segundo espera, reavalia o `WHERE` contra a linha já commitada, vê que ela não está mais pendente e não altera nada. Não existe lock em Go nem leitura seguida de escrita na aplicação. Os testes rodam 50 cancelamentos simultâneos da mesma reserva (exatamente um dá certo e o estoque volta uma vez só) e confirmar contra cancelar vinte vezes (sempre exatamente um vencedor).

Reservar e cancelar mexem em duas tabelas, então cada um é uma CTE que modifica dados: a atualização do estoque e a mudança da reserva são commitadas juntas ou não são, numa única ida ao banco.

## Por que o relógio do banco

Confirmar verifica `expires_at > now()` e o expirador verifica `expires_at <= now()`, os dois em SQL. Com o relógio de cada instância, duas instâncias com alguns segundos de diferença poderiam discordar: uma aceita a confirmação enquanto a outra já devolveu as unidades. Com `now()` existe um relógio só, e confirmar e expirar nunca ganham os dois. Um teste dispara uma confirmação e um `ExpireDue` na mesma reserva, a partir de dois pools e perto do `expires_at` dela, vinte vezes: ou a confirmação vence e nada expira, ou a confirmação é recusada e as unidades voltam exatamente uma vez.

## O expirador

`ExpireDue` pega um lote de reservas vencidas, marca como `expired` e devolve as unidades, num único comando:

1. `FOR UPDATE SKIP LOCKED` escolhe até `limit` linhas vencidas. Toda instância roda o expirador; o `SKIP LOCKED` faz cada uma pegar as linhas que as outras não travaram em vez de esperar por elas, então elas dividem o trabalho em vez de fazer fila. No teste com duas instâncias, cada reserva expirada aparece no log de uma instância só.
2. O limite do lote mantém cada transação curta, mesmo quando uma queda deixa milhares de reservas vencidas. O worker chama de novo na hora enquanto voltam lotes cheios, e depois espera o próximo tick.
3. As quantidades devolvidas são somadas por SKU, e as linhas de produto são travadas em ordem de SKU antes da atualização.

O último ponto veio de um teste. Oito expiradores rodando ao mesmo tempo sobre cinco SKUs entraram em deadlock: um lote segurava `PS5` e esperava `XBOX`, outro segurava `XBOX` e esperava `PS5`. Em 30 execuções o Postgres acusou 192 `deadlock detected`. Travar os produtos com `ORDER BY sku FOR UPDATE` faz toda transação pegar os locks na mesma ordem, e a contagem foi a zero.

## O worker e o desligamento

O worker chama `ExpireDue` ao subir, para recuperar o atraso depois de um tempo fora do ar, e depois a cada `EXPIRY_INTERVAL`. Um lote que falha vai para o log e é tentado de novo no próximo tick; não derruba o processo. Os testes usam `testing/synctest`, então um ticker de minutos roda na hora e sem depender de timing.

No `SIGINT` ou `SIGTERM`, o `http.Server.Shutdown` deixa as requisições em andamento terminarem, o worker para, e só então o pool fecha. Fechar o pool antes faria essas requisições e o lote em andamento falharem no meio. Um lote interrompido não é problema: a transação dá rollback e as linhas continuam vencidas para a próxima instância.

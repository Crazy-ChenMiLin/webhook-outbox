# webhook-outbox

Webhooks to your customers, queued in your application's database transaction and
delivered by a separate relay. Signed per [Standard Webhooks](https://www.standardwebhooks.com/),
retried on a schedule, with a log of every attempt. One PostgreSQL, nothing else.

> Status: in development, nothing is released yet.

## Why a table and not an API call

A webhook sent after the commit is a second write to a second system. The commit can
succeed and the call fail, or the call can go out for a transaction that rolls back.
Here the message is a row inserted in the same transaction as the order it describes:
it exists if and only if the order does. See
[ADR 0001](docs/adr/0001-messages-are-queued-in-the-application-transaction.md).

## Docs

- [Schema and endpoints](docs/schema.md)
- [Relay: settings, retries, guarantees](docs/relay.md)

## Layout

- The PHP package `ianfoxdev/webhook-outbox` is at the root: it queues messages and
  verifies signatures on the receiving side.
- The relay is a Go program in [relay/](relay), shipped as a Docker image.

One version number covers both.

## License

MIT

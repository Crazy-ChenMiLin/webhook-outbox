# 0001. Messages are queued in the application's transaction

Date: 2026-11-19. Status: accepted.

## Context

An application that sends webhooks to its customers usually has two writes for one
business event: the order is saved in its database, and the webhook is handed to a
sender. The usual sender is a service with an HTTP API: the application calls it after
the commit.

Two writes to two systems can disagree:

- The commit succeeds and the call fails, or the process dies between them. The order is
  paid, and the customer's system never hears about it.
- The call is made before the commit, and the transaction rolls back. The customer gets a
  webhook for an order that does not exist.
- The call times out after the sender stored the message. A retry sends it twice, under
  two ids, so the receiver cannot drop the copy.

This is the problem the transactional outbox solves for events going to a broker. A
webhook is the same event going to a URL.

## Decision

The application queues a webhook by inserting a row into `webhook_messages` in its own
database, inside the transaction that changes the business data. The PHP package does
the insert through the connection the application already uses. The relay, a separate
Go process, reads new rows from the same database and delivers them.

- The message exists if and only if the transaction committed. No webhook for a rolled
  back order, no lost webhook for a committed one.
- The message id is stored in the row and sent as `webhook-id` on every attempt, so the
  receiver can drop the copies the relay sends after a timeout.
- The relay claims work with `SELECT ... FOR UPDATE SKIP LOCKED`, so several relay
  processes can run at once without a leader.
- Only PostgreSQL in 0.1. MySQL 8 has `SKIP LOCKED` too and can follow when someone
  asks for it.

An HTTP API for queueing messages is left out of 0.1. Applications that are not written
in PHP can insert the row themselves: the table layout is documented and covered by
the compatibility promise.

## Consequences

- The relay needs a connection to the application's database. It should use its own role
  with access to the `webhook_` tables only.
- Delivery starts on the next poll, not at once. The poll interval is a setting; one
  second costs one indexed query per second on an idle system.
- The tables live in the application's database and grow with it. Delivered messages are
  deleted after a retention period, which is a setting.
- The schema is a contract between two languages: a change lands in the migration, the
  PHP writer and the relay query in the same release.

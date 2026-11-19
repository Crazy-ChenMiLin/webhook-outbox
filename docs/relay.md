# Relay

`webhook-outbox-relay run` delivers what the application queued. It needs
`DATABASE_URL` and nothing else; every other setting has a default.

| Variable | Default | Meaning |
|---|---|---|
| `RELAY_POLL_INTERVAL` | `1s` | wait after a round that found less than a full batch |
| `RELAY_BATCH_SIZE` | `100` | messages dispatched and deliveries claimed per round |
| `RELAY_CONCURRENCY` | `16` | requests in flight at once |
| `RELAY_TIMEOUT` | `15s` | limit for one request, from connecting to reading the response |
| `RELAY_RESPONSE_LIMIT` | `4096` | bytes of a response body kept in `webhook_attempts` |
| `RELAY_RETRY_SCHEDULE` | `5s,5m,30m,2h,5h,10h,10h` | pause after each failed attempt; `none` for one attempt |

Logs are JSON on stderr. SIGTERM finishes the requests in flight, records them and
exits, so a stop takes up to `RELAY_TIMEOUT`.

## What a round does

1. **Dispatch.** Up to a batch of new messages (`dispatched_at IS NULL`) is locked with
   `FOR UPDATE SKIP LOCKED`. For each, one delivery is created per enabled endpoint of
   the message's customer that takes its type, and the message is marked dispatched.
   One statement, one transaction.
2. **Claim.** Up to a batch of pending deliveries that are due and whose endpoint is
   enabled is locked the same way, and their `next_attempt_at` is moved past the end of
   the request (timeout plus 30 seconds). The transaction commits before any request is
   made, so no lock is held while waiting for a customer's server.
3. **Send.** A POST with the payload as the body, `Content-Type: application/json` and
   the three Standard Webhooks headers. `webhook-id` is the message id.
4. **Record.** The attempt is inserted and the delivery moved on in one transaction.

## Outcomes

| Response | Delivery |
|---|---|
| 2xx | `succeeded` |
| anything else, a timeout, a refused connection | `pending`, due again after the next pause of the schedule, with up to 10% jitter |
| the same after the last pause | `failed` |

A redirect is not followed: it counts as a failed attempt with its status code, and
the endpoint's URL should be updated.

## Guarantees and limits

- **At least once.** A relay that dies after sending and before recording leaves the
  delivery claimed until the lease runs out; then it is sent again under the same
  `webhook-id`. Receivers drop repeats by that id.
- **No order.** Two messages to one endpoint can arrive in any order, and a retried
  message arrives after newer ones. Put a version or a time in the payload if the
  receiver needs order.
- **Several relays** can run against one database. They never take the same row at
  once; there is no leader.
- **Endpoints added later** get only messages dispatched after they were added.
  **Disabled endpoints** get no new deliveries; their pending ones wait and go out after
  `endpoints enable`.

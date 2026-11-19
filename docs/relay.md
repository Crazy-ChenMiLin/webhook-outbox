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
| `RELAY_DISABLE_AFTER` | `5d` | disable an endpoint when no attempt has succeeded for this long (`36h`, `7d`); `never` to keep it |
| `RELAY_ALLOW_PRIVATE_NETWORKS` | `false` | `true` lets the relay send to loopback, private and link-local addresses |

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

## Disabled endpoints

The relay disables an endpoint in two cases, and logs `endpoint disabled` with the
reason:

- **410 Gone.** The receiver says the URL is gone. The endpoint is disabled after that
  one attempt, with the reason `the endpoint answered 410 Gone`.
- **A failure streak.** The first failed attempt after a success sets
  `failing_since`; any success clears it. A failed attempt when `failing_since` is older
  than `RELAY_DISABLE_AFTER` disables the endpoint with the reason
  `no successful attempt since <time>`. With the default schedule a delivery gives up
  after about 28 hours, so a dead endpoint is disabled by the messages that keep coming
  after that, not by one old message.

A disabled endpoint gets no new deliveries. Its pending ones keep their attempt count
and wait; a delivery whose last attempt was the one that disabled the endpoint is
still `pending`. `endpoints enable` starts a new streak and the waiting deliveries go
out in the next round. Messages queued while the endpoint was disabled are not sent to
it.

## Private addresses

Endpoint URLs come from customers, so by default the relay refuses to connect to:

| Range | Examples |
|---|---|
| loopback | `127.0.0.0/8`, `::1` |
| private networks | `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7` |
| link-local, where cloud metadata lives | `169.254.0.0/16` (`169.254.169.254`), `fe80::/10` |
| carrier-grade NAT, reserved, documentation, benchmarking | `100.64.0.0/10`, `240.0.0.0/4`, `192.0.2.0/24`, `198.18.0.0/15` |
| unspecified, multicast, broadcast | `0.0.0.0/8`, `::`, `224.0.0.0/4`, `ff00::/8` |

IPv4-mapped IPv6 (`::ffff:10.0.0.1`) and NAT64 (`64:ff9b::a9fe:a9fe`) addresses are
checked as the IPv4 address inside. The check runs on the address the relay is about
to connect to, after DNS. A name that resolves to `10.0.0.5` is refused, also when it
resolved to a public address a moment before (DNS rebinding). The attempt is recorded
with no status code and an error such as `blocked: 127.0.0.1 is not a public address
(loopback)`, and retried like any failure.

The relay does not use `HTTP_PROXY` or `HTTPS_PROXY`: through a proxy, the address it
connects to is the proxy's, and the check would mean nothing.

Set `RELAY_ALLOW_PRIVATE_NETWORKS=true` only when every endpoint URL is yours, for
example when the receivers are your own services in the same network.

## Guarantees and limits

- **At least once.** A relay that dies after sending and before recording leaves the
  delivery claimed until the lease runs out; then it is sent again under the same
  `webhook-id`. Receivers drop repeats by that id.
- **No order.** Two messages to one endpoint can arrive in any order, and a retried
  message arrives after newer ones. Put a version or a time in the payload if the
  receiver needs order.
- **Several relays** can run against one database. They never take the same row at
  once; there is no leader.
- **Limits.** A request takes at most `RELAY_TIMEOUT`, a response body is read up to
  `RELAY_RESPONSE_LIMIT` bytes, and redirects are not followed. A slow endpoint can
  hold up to `RELAY_CONCURRENCY` requests in flight for that long; there is no limit
  per endpoint yet.
- **Endpoints added later** get only messages dispatched after they were added.
  **Disabled endpoints** get no new deliveries; their pending ones wait and go out after
  `endpoints enable`.

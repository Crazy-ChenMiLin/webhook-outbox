<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox;

use IanFoxDev\WebhookOutbox\Exception\InvalidArgument;
use IanFoxDev\WebhookOutbox\Exception\NoActiveTransaction;

/**
 * Queues webhooks in the application's PostgreSQL transaction. The relay
 * (ghcr.io/ianfoxdev/webhook-outbox-relay) delivers them after the commit.
 *
 * Pass the PDO connection the application writes with. Laravel:
 * DB::connection()->getPdo(); Doctrine DBAL with pdo_pgsql:
 * $connection->getNativeConnection(). Transactions opened through either are PDO
 * transactions, so send() sees them.
 */
final readonly class WebhookOutbox implements Sender
{
    /** Ids go into a header, so they stay printable and short. */
    private const ID_PATTERN = '/^[A-Za-z0-9_.:-]{1,128}$/';

    /**
     * @param \Closure(): \DateTimeImmutable|null $clock for the body's timestamp; tests pass a fixed one
     */
    public function __construct(
        private \PDO $pdo,
        private ?\Closure $clock = null,
    ) {
        $driver = $pdo->getAttribute(\PDO::ATTR_DRIVER_NAME);
        if ($driver !== 'pgsql') {
            throw new InvalidArgument(sprintf('Only PostgreSQL is supported, got the %s PDO driver.', is_string($driver) ? $driver : 'unknown'));
        }
    }

    /**
     * Queues a webhook in the current transaction. The body is a Standard Webhooks
     * payload: {"type": $type, "timestamp": now, "data": $data}.
     *
     * A message id that exists already fails the insert with a unique violation, which
     * aborts the transaction: pass your own id only when a repeat is a bug.
     *
     * @param array<mixed>|\JsonSerializable $data
     *
     * @throws NoActiveTransaction when called outside a transaction
     * @throws InvalidArgument     for an empty customer or type, a malformed id, or data that is not JSON
     */
    public function send(string $customer, string $type, array|\JsonSerializable $data, ?string $id = null): string
    {
        if (!$this->pdo->inTransaction()) {
            throw new NoActiveTransaction('WebhookOutbox::send() must run inside the transaction that changes the state.');
        }
        if (trim($customer) === '' || trim($type) === '') {
            throw new InvalidArgument('Customer and type must not be empty.');
        }
        if ($id !== null && preg_match(self::ID_PATTERN, $id) !== 1) {
            throw new InvalidArgument(sprintf('Message id %s must be 1 to 128 characters of A-Z, a-z, 0-9 and _ . : -.', json_encode($id)));
        }
        $id ??= MessageId::generate();

        $now = $this->clock !== null ? ($this->clock)() : new \DateTimeImmutable();
        try {
            $payload = json_encode(
                [
                    'type' => $type,
                    'timestamp' => $now->setTimezone(new \DateTimeZone('UTC'))->format('Y-m-d\TH:i:s.v\Z'),
                    // Standard Webhooks wants "data" to be an object; PHP's [] would encode as a list.
                    'data' => $data === [] ? new \stdClass() : $data,
                ],
                JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_PRESERVE_ZERO_FRACTION,
            );
        } catch (\JsonException $e) {
            throw new InvalidArgument('Data cannot be encoded as JSON: ' . $e->getMessage(), 0, $e);
        }

        // With PDO::ERRMODE_SILENT a failed insert only returns false; a webhook that
        // was not queued must not look like one that was.
        $statement = $this->pdo->prepare('INSERT INTO webhook_messages (id, customer, type, payload) VALUES (?, ?, ?, ?)');
        if ($statement === false || !$statement->execute([$id, $customer, $type, $payload])) {
            $info = $statement === false ? $this->pdo->errorInfo() : $statement->errorInfo();
            $state = isset($info[0]) && is_string($info[0]) ? $info[0] : '?';
            $message = isset($info[2]) && is_string($info[2]) ? $info[2] : '';
            throw new \PDOException(sprintf('Queueing the webhook failed: SQLSTATE[%s] %s', $state, $message));
        }

        return $id;
    }
}

<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Testing;

use IanFoxDev\WebhookOutbox\MessageId;
use IanFoxDev\WebhookOutbox\Sender;

/**
 * A Sender for unit tests: keeps what was sent in memory, with no database and no
 * transaction.
 */
final class InMemorySender implements Sender
{
    /** @var list<array{id: string, customer: string, type: string, data: array<mixed>|\JsonSerializable}> */
    public array $sent = [];

    public function send(string $customer, string $type, array|\JsonSerializable $data, ?string $id = null): string
    {
        $id ??= MessageId::generate();
        $this->sent[] = ['id' => $id, 'customer' => $customer, 'type' => $type, 'data' => $data];

        return $id;
    }

    /**
     * @return list<array{id: string, customer: string, type: string, data: array<mixed>|\JsonSerializable}>
     */
    public function ofType(string $type): array
    {
        return array_values(array_filter($this->sent, static fn (array $m): bool => $m['type'] === $type));
    }
}

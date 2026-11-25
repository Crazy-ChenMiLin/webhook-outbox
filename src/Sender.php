<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox;

/**
 * What application code depends on to queue webhooks.
 *
 * WebhookOutbox writes to the database. Type-hint this interface in your services, and
 * a unit test can pass Testing\InMemorySender instead.
 */
interface Sender
{
    /**
     * Queues a webhook in the current transaction. It goes out only if the transaction
     * commits, to every enabled endpoint of the customer that takes the type.
     *
     * @param string                         $customer your id for whoever owns the endpoints
     * @param string                         $type     event type, for example "order.paid"
     * @param array<mixed>|\JsonSerializable $data     the "data" member of the body
     * @param string|null                    $id       message id, sent as webhook-id; generated when null
     *
     * @return string the message id
     */
    public function send(string $customer, string $type, array|\JsonSerializable $data, ?string $id = null): string;
}

<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Tests\Unit;

use IanFoxDev\WebhookOutbox\Testing\InMemorySender;
use PHPUnit\Framework\TestCase;

final class InMemorySenderTest extends TestCase
{
    public function testKeepsWhatWasSent(): void
    {
        $sender = new InMemorySender();
        $id = $sender->send('acme', 'order.paid', ['order' => 'A-17']);
        $sender->send('acme', 'order.refunded', ['order' => 'A-17'], 'refund-A-17');

        self::assertStringStartsWith('msg_', $id);
        self::assertSame([['id' => $id, 'customer' => 'acme', 'type' => 'order.paid', 'data' => ['order' => 'A-17']]], $sender->ofType('order.paid'));
        self::assertSame('refund-A-17', $sender->sent[1]['id']);
    }
}

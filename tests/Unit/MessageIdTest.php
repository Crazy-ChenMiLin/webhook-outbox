<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Tests\Unit;

use IanFoxDev\WebhookOutbox\MessageId;
use PHPUnit\Framework\TestCase;

final class MessageIdTest extends TestCase
{
    public function testFormat(): void
    {
        self::assertMatchesRegularExpression('/^msg_[0-9A-HJKMNP-TV-Z]{26}$/', MessageId::generate());
    }

    public function testTimePrefix(): void
    {
        // The ULID spec's example time, 1469918176385 ms, encodes as 01ARYZ6S41.
        self::assertStringStartsWith('msg_01ARYZ6S41', MessageId::generate(1469918176385));
    }

    public function testSortsByTime(): void
    {
        $earlier = MessageId::generate(1790330400000);
        $later = MessageId::generate(1790330400001);

        self::assertLessThan(0, strcmp($earlier, $later));
    }

    public function testUnique(): void
    {
        $ids = [];
        for ($i = 0; $i < 1000; ++$i) {
            $ids[MessageId::generate(1790330400000)] = true;
        }

        self::assertCount(1000, $ids);
    }
}

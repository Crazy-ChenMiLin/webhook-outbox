<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Tests\Unit;

use IanFoxDev\WebhookOutbox\Exception\InvalidArgument;
use IanFoxDev\WebhookOutbox\WebhookOutbox;
use PHPUnit\Framework\Attributes\RequiresPhpExtension;
use PHPUnit\Framework\TestCase;

final class WebhookOutboxTest extends TestCase
{
    #[RequiresPhpExtension('pdo_sqlite')]
    public function testOtherDriversAreRefused(): void
    {
        $this->expectException(InvalidArgument::class);
        $this->expectExceptionMessage('got the sqlite PDO driver');

        new WebhookOutbox(new \PDO('sqlite::memory:'));
    }
}

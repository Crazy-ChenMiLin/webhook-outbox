<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Exception;

/**
 * send() was called outside a transaction. Outside one, the webhook could go out for
 * a change that is rolled back, or be lost for one that is committed.
 */
final class NoActiveTransaction extends \LogicException implements WebhookOutboxException
{
}

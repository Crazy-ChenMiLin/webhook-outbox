<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Exception;

final class InvalidArgument extends \InvalidArgumentException implements WebhookOutboxException
{
}

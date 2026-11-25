<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Exception;

/**
 * A received webhook is not signed with the endpoint's secret, is too old or too new,
 * or misses a header. Answer 400 or 401 and do not act on it.
 */
final class VerificationFailed extends \RuntimeException implements WebhookOutboxException
{
}

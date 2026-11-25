<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox;

/**
 * Generates message ids: "msg_" and a ULID, 48 bits of milliseconds and 80 random bits
 * in Crockford base32. Ids sort by creation time, which makes the tables easier to read.
 */
final class MessageId
{
    private const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

    public static function generate(?int $milliseconds = null): string
    {
        $milliseconds ??= (int) floor(microtime(true) * 1000);
        $time = '';
        for ($i = 0; $i < 10; ++$i) {
            $time = self::ALPHABET[$milliseconds % 32] . $time;
            $milliseconds = intdiv($milliseconds, 32);
        }

        // 80 bits are 16 characters of 5 bits.
        $bits = '';
        foreach (str_split(random_bytes(10)) as $byte) {
            $bits .= str_pad(decbin(ord($byte)), 8, '0', STR_PAD_LEFT);
        }
        $random = '';
        foreach (str_split($bits, 5) as $chunk) {
            $random .= self::ALPHABET[(int) bindec($chunk)];
        }

        return 'msg_' . $time . $random;
    }
}

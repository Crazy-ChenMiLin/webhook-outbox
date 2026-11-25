<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox;

use IanFoxDev\WebhookOutbox\Exception\InvalidArgument;
use IanFoxDev\WebhookOutbox\Exception\VerificationFailed;

/**
 * The receiving side: checks the signature of a webhook, per Standard Webhooks. It
 * works for any sender that follows the spec, not only this relay.
 *
 *     $webhook = new Webhook($secretFromYourConfig);
 *     $event = $webhook->verify($request->getContent(), $request->headers->all());
 *
 * Pass the raw body, before any JSON decoding: the signature covers its exact bytes.
 */
final readonly class Webhook
{
    private string $key;

    /**
     * @param string $secret the endpoint secret, "whsec_" and base64; the prefix is optional
     * @param int    $tolerance how far, in seconds, webhook-timestamp may be from now, both ways
     */
    public function __construct(string $secret, private int $tolerance = 300)
    {
        $encoded = str_starts_with($secret, 'whsec_') ? substr($secret, 6) : $secret;
        $key = base64_decode($encoded, true);
        if ($key === false || $key === '') {
            throw new InvalidArgument('The secret is not "whsec_" followed by base64.');
        }
        if ($tolerance < 1) {
            throw new InvalidArgument('Tolerance must be at least one second.');
        }
        $this->key = $key;
    }

    /**
     * Checks the headers and returns the decoded body.
     *
     * Header names are matched in any case. A value may be a string or a list of
     * strings, as PSR-7, Symfony and Laravel return them. webhook-signature may carry
     * several space-separated signatures, during a secret rotation for example; one
     * valid v1 signature is enough.
     *
     * @param array<string, string|list<string>> $headers
     *
     * @return array<mixed>
     *
     * @throws VerificationFailed
     */
    public function verify(string $body, array $headers, ?int $now = null): array
    {
        $id = self::header($headers, 'webhook-id');
        $timestamp = self::header($headers, 'webhook-timestamp');
        $signatures = self::header($headers, 'webhook-signature');

        if (preg_match('/^\d{1,12}$/', $timestamp) !== 1) {
            throw new VerificationFailed('webhook-timestamp is not a Unix time in seconds.');
        }
        $now ??= time();
        if (abs($now - (int) $timestamp) > $this->tolerance) {
            throw new VerificationFailed(sprintf('webhook-timestamp is %d seconds away from now, more than %d.', abs($now - (int) $timestamp), $this->tolerance));
        }

        $expected = $this->signature($id, (int) $timestamp, $body);
        $valid = false;
        foreach (explode(' ', $signatures) as $signature) {
            // Compare every candidate, so the time taken does not depend on which matched.
            if (str_starts_with($signature, 'v1,') && hash_equals($expected, $signature)) {
                $valid = true;
            }
        }
        if (!$valid) {
            throw new VerificationFailed('No valid v1 signature in webhook-signature.');
        }

        try {
            $decoded = json_decode($body, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $e) {
            throw new VerificationFailed('The body is signed but is not JSON: ' . $e->getMessage(), 0, $e);
        }
        if (!is_array($decoded)) {
            throw new VerificationFailed('The body is signed but is not a JSON object.');
        }

        return $decoded;
    }

    /**
     * Signs a body the way the relay does. For tests of your receiver.
     *
     * @return array{webhook-id: string, webhook-timestamp: string, webhook-signature: string}
     */
    public function sign(string $id, int $timestamp, string $body): array
    {
        return [
            'webhook-id' => $id,
            'webhook-timestamp' => (string) $timestamp,
            'webhook-signature' => $this->signature($id, $timestamp, $body),
        ];
    }

    private function signature(string $id, int $timestamp, string $body): string
    {
        return 'v1,' . base64_encode(hash_hmac('sha256', $id . '.' . $timestamp . '.' . $body, $this->key, true));
    }

    /**
     * @param array<string, string|list<string>> $headers
     */
    private static function header(array $headers, string $name): string
    {
        foreach ($headers as $key => $value) {
            if (strtolower((string) $key) !== $name) {
                continue;
            }
            $value = is_array($value) ? implode(' ', $value) : $value;
            if (trim($value) === '') {
                break;
            }

            return trim($value);
        }

        throw new VerificationFailed(sprintf('The %s header is missing.', $name));
    }
}

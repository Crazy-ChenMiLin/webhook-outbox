<?php

declare(strict_types=1);

// End-to-end check of the example: places orders through the shop, waits for the
// relay, then compares what each receiver endpoint got with the delivery log.
// Run with `docker compose run --rm e2e`; exits 1 on the first failed check.

const SHOP = 'http://shop:8000';
const RECEIVER = 'http://receiver:8000';
const RELAY = 'http://relay:8080';

$pdo = new PDO((string) getenv('DATABASE_DSN'), options: [PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION]);

/** @return array{int, array<string, mixed>} */
function post(string $url, array $form): array
{
    $context = stream_context_create(['http' => [
        'method' => 'POST',
        'header' => 'Content-Type: application/x-www-form-urlencoded',
        'content' => http_build_query($form),
        'ignore_errors' => true,
    ]]);
    $body = file_get_contents($url, false, $context);
    preg_match('{^HTTP/\S+ (\d+)}', $http_response_header[0], $m);

    return [(int) $m[1], json_decode((string) $body, true, flags: JSON_THROW_ON_ERROR)];
}

function check(bool $ok, string $what): void
{
    echo ($ok ? 'ok   ' : 'FAIL ').$what."\n";
    if (!$ok) {
        exit(1);
    }
}

/** @return array<string, mixed>|false */
function delivery(PDO $pdo, string $message): array|false
{
    $q = $pdo->prepare('SELECT d.id, d.status, d.attempts, e.disabled_at, e.disabled_reason
        FROM webhook_deliveries d JOIN webhook_endpoints e ON e.id = d.endpoint_id
        WHERE d.message_id = ?');
    $q->execute([$message]);

    return $q->fetch(PDO::FETCH_ASSOC);
}

/** @return list<array<string, mixed>> */
function attempts(PDO $pdo, int $delivery): array
{
    $q = $pdo->prepare('SELECT status_code, error, duration_ms FROM webhook_attempts WHERE delivery_id = ? ORDER BY id');
    $q->execute([$delivery]);

    return $q->fetchAll(PDO::FETCH_ASSOC);
}

$count = static fn (string $table): int => (int) $pdo->query("SELECT count(*) FROM $table")->fetchColumn();
// The checks count attempts and metrics from zero, and /gone stays disabled after a run.
check($count('orders') === 0, 'a fresh stack (docker compose down -v, then up)');

// One order per scenario. The customer is the scenario, see shop/setup.php.
$messages = [];
foreach (['ok', 'slow', 'flaky', 'timeout', 'down', 'gone'] as $scenario) {
    [$status, $body] = post(SHOP.'/orders', ['customer' => $scenario, 'total' => 1999]);
    check($status === 201, "order for $scenario accepted");
    $messages[$scenario] = $body['message'];
}

// A checkout that fails after the insert leaves neither the order nor the message.
[$orders, $queued] = [$count('orders'), $count('webhook_messages')];
[$status] = post(SHOP.'/orders', ['customer' => 'ok', 'total' => 500, 'fail' => 1]);
check($status === 503, 'failed checkout answers 503');
check($count('orders') === $orders && $count('webhook_messages') === $queued, 'failed checkout left no order and no message');

// Wait until every delivery is settled. With a 1s,1s schedule and a 2s timeout the
// slowest, /timeout, takes three timeouts and two pauses: about 8 seconds.
$settled = static function () use ($pdo, $messages): bool {
    foreach ($messages as $scenario => $message) {
        $d = delivery($pdo, $message);
        $done = $scenario === 'gone' ? ($d !== false && $d['disabled_at'] !== null) : ($d !== false && $d['status'] !== 'pending');
        if (!$done) {
            return false;
        }
    }

    return true;
};
for ($deadline = time() + 30; !$settled() && time() < $deadline;) {
    usleep(250_000);
}
check($settled(), 'every delivery settled within 30 seconds');

$received = json_decode((string) file_get_contents(RECEIVER.'/log'), true, flags: JSON_THROW_ON_ERROR);
$got = static fn (string $scenario): array => array_values(array_filter($received, static fn (array $r): bool => $r['scenario'] === $scenario));
$codes = static fn (array $attempts): array => array_map(static fn (array $a): ?int => $a['status_code'], $attempts);

check(array_filter($received, static fn (array $r): bool => !$r['verified']) === [], 'receiver verified the signature of every request');
check(array_filter($received, static fn (array $r): bool => $r['id'] !== $messages[$r['scenario']]) === [], 'every request carried the message id from send() as webhook-id');

$ok = $got('ok');
check(count($ok) === 1 && $ok[0]['event']['type'] === 'order.placed' && $ok[0]['event']['data']['total'] === 1999, 'ok: order.placed arrived once with the order');
check(delivery($pdo, $messages['ok'])['status'] === 'succeeded', 'ok: delivery succeeded');

$d = delivery($pdo, $messages['slow']);
$a = attempts($pdo, $d['id']);
check($d['status'] === 'succeeded' && count($a) === 1 && $a[0]['duration_ms'] >= 1000, 'slow: one attempt of a second or more, succeeded');

$d = delivery($pdo, $messages['flaky']);
check($d['status'] === 'succeeded' && $codes(attempts($pdo, $d['id'])) === [500, 500, 200], 'flaky: 500, 500, then 200');

$d = delivery($pdo, $messages['timeout']);
$a = attempts($pdo, $d['id']);
check($d['status'] === 'failed' && $codes($a) === [null, null, null], 'timeout: three attempts without a response, failed');
check(str_contains((string) $a[0]['error'], 'Timeout') || str_contains((string) $a[0]['error'], 'deadline'), 'timeout: attempt error names the timeout');

$d = delivery($pdo, $messages['down']);
check($d['status'] === 'failed' && $codes(attempts($pdo, $d['id'])) === [500, 500, 500], 'down: 500 three times, failed');

$d = delivery($pdo, $messages['gone']);
check($d['status'] === 'pending' && $codes(attempts($pdo, $d['id'])) === [410], 'gone: one attempt, delivery waits');
check(str_contains((string) $d['disabled_reason'], '410'), 'gone: endpoint disabled for the 410');

$metrics = (string) file_get_contents(RELAY.'/metrics');
check(str_contains($metrics, 'webhook_outbox_attempts_total{code="2xx",result="succeeded"} 3'), 'relay counts the three successful attempts on /metrics');
check(str_contains($metrics, 'webhook_outbox_endpoints_disabled_total{reason="gone"} 1'), 'relay counts the disabled endpoint on /metrics');

echo "all checks passed\n";

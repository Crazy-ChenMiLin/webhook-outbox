<?php

declare(strict_types=1);

// A customer's receiver. Each path is one way an endpoint behaves:
//
//   /ok       answers 200
//   /slow     answers 200 after 1 second, inside the relay's timeout
//   /flaky    answers 500 to the first two attempts of a message, then 200
//   /timeout  answers after 5 seconds, past the relay's timeout
//   /down     always answers 500
//   /gone     answers 410: the customer removed the endpoint
//
// Every request is checked with Webhook::verify() and logged; GET /log returns the log.

use IanFoxDev\WebhookOutbox\Exception\VerificationFailed;
use IanFoxDev\WebhookOutbox\Webhook;

require __DIR__.'/../vendor/autoload.php';

const LOG = '/tmp/receiver.log';

$path = trim((string) parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH), '/');
if ($path === 'health') {
    echo 'ok';

    return;
}
if ($path === 'log') {
    header('Content-Type: application/json');
    $lines = is_file(LOG) ? file(LOG, FILE_IGNORE_NEW_LINES) : [];
    echo '[', implode(',', $lines), "]\n";

    return;
}

$secrets = json_decode((string) file_get_contents('/secrets/secrets.json'), true, flags: JSON_THROW_ON_ERROR);
if (!isset($secrets[$path])) {
    http_response_code(404);

    return;
}

$body = (string) file_get_contents('php://input');
$headers = getallheaders();
$entry = ['scenario' => $path, 'id' => $headers['Webhook-Id'] ?? $headers['webhook-id'] ?? null];
try {
    $entry['event'] = (new Webhook($secrets[$path]))->verify($body, $headers);
    $entry['verified'] = true;
} catch (VerificationFailed $e) {
    $entry['verified'] = false;
    $entry['error'] = $e->getMessage();
}

match ($path) {
    'slow' => sleep(1),
    'timeout' => sleep(5),
    default => null,
};
$status = match ($path) {
    'ok', 'slow', 'timeout' => 200,
    'flaky' => attempt((string) $entry['id']) <= 2 ? 500 : 200,
    'down' => 500,
    'gone' => 410,
};
if (!$entry['verified']) {
    $status = 400;
}
$entry['status'] = $status;
file_put_contents(LOG, json_encode($entry, JSON_THROW_ON_ERROR)."\n", FILE_APPEND | LOCK_EX);

http_response_code($status);
echo $status < 300 ? "thanks\n" : "no\n";

// attempt counts the requests seen for one message id, across server workers.
function attempt(string $id): int
{
    $file = '/tmp/attempts-'.hash('sha256', $id);
    $f = fopen($file, 'c+');
    flock($f, LOCK_EX);
    $n = (int) stream_get_contents($f) + 1;
    ftruncate($f, 0);
    rewind($f);
    fwrite($f, (string) $n);
    flock($f, LOCK_UN);
    fclose($f);

    return $n;
}

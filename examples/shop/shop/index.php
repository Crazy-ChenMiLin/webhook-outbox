<?php

declare(strict_types=1);

// The shop: POST /orders creates an order and queues order.placed in one transaction.

use IanFoxDev\WebhookOutbox\WebhookOutbox;

require __DIR__.'/../vendor/autoload.php';

function respond(int $status, array $body): void
{
    http_response_code($status);
    header('Content-Type: application/json');
    echo json_encode($body, JSON_THROW_ON_ERROR), "\n";
}

$path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
if ($path === '/health') {
    echo 'ok';

    return;
}
if ($path !== '/orders' || $_SERVER['REQUEST_METHOD'] !== 'POST') {
    respond(404, ['error' => 'not found']);

    return;
}

$customer = (string) ($_POST['customer'] ?? '');
$total = filter_var($_POST['total'] ?? null, FILTER_VALIDATE_INT, ['options' => ['min_range' => 1]]);
if ($customer === '' || $total === false) {
    respond(422, ['error' => 'customer and a positive integer total are required']);

    return;
}

$pdo = new PDO((string) getenv('DATABASE_DSN'), options: [PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION]);
$webhooks = new WebhookOutbox($pdo);

$pdo->beginTransaction();
try {
    $insert = $pdo->prepare('INSERT INTO orders (customer, total) VALUES (?, ?) RETURNING id');
    $insert->execute([$customer, $total]);
    $order = (int) $insert->fetchColumn();
    $message = $webhooks->send($customer, 'order.placed', ['order' => $order, 'total' => $total]);
    if (isset($_POST['fail'])) {
        // What a timed out payment call after the insert would do.
        throw new RuntimeException('payment provider timed out');
    }
    $pdo->commit();
} catch (Throwable $e) {
    $pdo->rollBack();
    respond(503, ['error' => $e->getMessage()]);

    return;
}

respond(201, ['order' => $order, 'message' => $message]);

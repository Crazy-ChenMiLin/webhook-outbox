<?php

declare(strict_types=1);

namespace IanFoxDev\WebhookOutbox\Tests\Integration;

use IanFoxDev\WebhookOutbox\Exception\InvalidArgument;
use IanFoxDev\WebhookOutbox\Exception\NoActiveTransaction;
use IanFoxDev\WebhookOutbox\WebhookOutbox;
use PHPUnit\Framework\TestCase;

/**
 * Runs against the PostgreSQL in WEBHOOK_OUTBOX_PG_DSN, each test in a schema of its
 * own made from schema/postgresql.sql.
 */
final class WebhookOutboxTest extends TestCase
{
    private \PDO $pdo;
    private string $schema;

    protected function setUp(): void
    {
        $dsn = getenv('WEBHOOK_OUTBOX_PG_DSN');
        if ($dsn === false || $dsn === '') {
            self::markTestSkipped('WEBHOOK_OUTBOX_PG_DSN is not set.');
        }
        $this->pdo = new \PDO($dsn, options: [\PDO::ATTR_ERRMODE => \PDO::ERRMODE_EXCEPTION]);
        $this->schema = 'php_' . bin2hex(random_bytes(6));
        $this->pdo->exec(sprintf('CREATE SCHEMA %1$s; SET search_path = %1$s', $this->schema));
        $sql = file_get_contents(__DIR__ . '/../../schema/postgresql.sql');
        self::assertIsString($sql);
        $this->pdo->exec($sql);
    }

    protected function tearDown(): void
    {
        if (isset($this->schema)) {
            if ($this->pdo->inTransaction()) {
                $this->pdo->rollBack();
            }
            $this->pdo->exec(sprintf('DROP SCHEMA %s CASCADE', $this->schema));
        }
    }

    private function outbox(): WebhookOutbox
    {
        return new WebhookOutbox($this->pdo, static fn (): \DateTimeImmutable => new \DateTimeImmutable('2026-11-25 12:30:00.250', new \DateTimeZone('Asia/Nicosia')));
    }

    /**
     * @return list<array{id: string, customer: string, type: string, payload: string, dispatched: bool}>
     */
    private function messages(): array
    {
        $statement = $this->pdo->query('SELECT id, customer, type, payload, dispatched_at IS NOT NULL AS dispatched FROM webhook_messages ORDER BY id');
        self::assertInstanceOf(\PDOStatement::class, $statement);

        /** @var list<array{id: string, customer: string, type: string, payload: string, dispatched: bool}> */
        return $statement->fetchAll(\PDO::FETCH_ASSOC);
    }

    public function testCommittedMessageIsQueued(): void
    {
        $this->pdo->beginTransaction();
        $this->pdo->exec("CREATE TABLE orders (id text PRIMARY KEY, status text); INSERT INTO orders VALUES ('A-17', 'paid')");
        $id = $this->outbox()->send('acme', 'order.paid', ['order' => 'A-17', 'amount' => 5000, 'note' => 'caf' . "\u{e9}" . ' / bar', 'rate' => 1.0]);
        $this->pdo->commit();

        self::assertMatchesRegularExpression('/^msg_[0-9A-Z]{26}$/', $id);
        self::assertSame([[
            'id' => $id,
            'customer' => 'acme',
            'type' => 'order.paid',
            'payload' => '{"type":"order.paid","timestamp":"2026-11-25T10:30:00.250Z","data":{"order":"A-17","amount":5000,"note":"caf' . "\u{e9}" . ' / bar","rate":1.0}}',
            'dispatched' => false,
        ]], $this->messages());
    }

    public function testRolledBackMessageIsGone(): void
    {
        $this->pdo->beginTransaction();
        $this->outbox()->send('acme', 'order.paid', ['order' => 'A-17']);
        $this->pdo->rollBack();

        self::assertSame([], $this->messages());
    }

    public function testOwnIdAndJsonSerializable(): void
    {
        $data = new class implements \JsonSerializable {
            public function jsonSerialize(): mixed
            {
                return ['order' => 'A-18'];
            }
        };
        $this->pdo->beginTransaction();
        $id = $this->outbox()->send('acme', 'order.refunded', $data, 'order:A-18:refunded');
        $this->pdo->commit();

        self::assertSame('order:A-18:refunded', $id);
        self::assertStringEndsWith('"data":{"order":"A-18"}}', $this->messages()[0]['payload']);
    }

    public function testEmptyDataIsAnObject(): void
    {
        $this->pdo->beginTransaction();
        $this->outbox()->send('acme', 'ping', []);
        $this->pdo->commit();

        self::assertStringEndsWith('"data":{}}', $this->messages()[0]['payload']);
    }

    public function testRepeatedIdFailsTheTransaction(): void
    {
        $this->pdo->beginTransaction();
        $this->outbox()->send('acme', 'order.paid', [], 'order:A-17:paid');

        $this->expectException(\PDOException::class);
        $this->expectExceptionMessage('23505');
        $this->outbox()->send('acme', 'order.paid', [], 'order:A-17:paid');
    }

    public function testOutsideATransaction(): void
    {
        $this->expectException(NoActiveTransaction::class);

        $this->outbox()->send('acme', 'order.paid', []);
    }

    public function testBadArguments(): void
    {
        $this->pdo->beginTransaction();
        foreach ([
            fn () => $this->outbox()->send(' ', 'order.paid', []),
            fn () => $this->outbox()->send('acme', '', []),
            fn () => $this->outbox()->send('acme', 'order.paid', [], "bad id\n"),
            fn () => $this->outbox()->send('acme', 'order.paid', ['x' => NAN]),
            fn () => $this->outbox()->send('acme', 'order.paid', ['x' => "\xff"]),
        ] as $i => $call) {
            try {
                $call();
                self::fail("call $i did not throw");
            } catch (InvalidArgument) {
            }
        }
        $this->pdo->commit();
        self::assertSame([], $this->messages());
    }

    public function testSilentErrorModeStillThrows(): void
    {
        $this->pdo->setAttribute(\PDO::ATTR_ERRMODE, \PDO::ERRMODE_SILENT);
        $this->pdo->beginTransaction();
        $this->outbox()->send('acme', 'order.paid', [], 'order:A-17:paid');

        $this->expectException(\PDOException::class);
        $this->expectExceptionMessage('SQLSTATE[23505]');
        $this->outbox()->send('acme', 'order.paid', [], 'order:A-17:paid');
    }
}

// Package replay sends messages again: to every endpoint that got them, or to one
// endpoint that missed them.
package replay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/endpoint"
)

// Result counts what a replay changed. Deliveries that are pending already are left
// as they are: they will go out anyway, and resetting one that is in flight would
// only send it twice.
type Result struct {
	// Requeued is how many succeeded or failed deliveries are pending again, with a
	// fresh retry schedule.
	Requeued int64
	// Created is how many deliveries were made for messages the endpoint never got:
	// it was disabled, or added later, when they were dispatched.
	Created int64
}

// ErrMessageNotFound is returned when one of the message ids does not exist.
var ErrMessageNotFound = errors.New("message not found")

const requeue = `UPDATE webhook_deliveries d
	SET status = 'pending', attempts = 0, next_attempt_at = now(), updated_at = now()`

// create makes the missing deliveries for the messages chosen by where, to endpoint
// $1, if it belongs to the message's customer and takes the type. Messages the relay
// has not dispatched yet are skipped: dispatch will see the endpoint itself.
const create = `INSERT INTO webhook_deliveries (message_id, endpoint_id)
	SELECT m.id, e.id FROM webhook_messages m
	JOIN webhook_endpoints e ON e.id = $1 AND e.customer = m.customer
		AND (e.event_types IS NULL OR m.type = ANY (e.event_types))
	WHERE m.dispatched_at IS NOT NULL AND `

// Messages requeues the deliveries of the given messages. With an endpoint id, only
// that endpoint's delivery is requeued, and created if the endpoint never got the
// message.
func Messages(ctx context.Context, conn *pgx.Conn, ids []string, endpointID string) (Result, error) {
	var res Result
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var found int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM webhook_messages WHERE id = ANY ($1)`, ids).Scan(&found); err != nil {
			return err
		}
		if found != len(unique(ids)) {
			return missing(ctx, tx, ids)
		}
		if endpointID == "" {
			tag, err := tx.Exec(ctx, requeue+` WHERE d.message_id = ANY ($1) AND d.status <> 'pending'`, ids)
			res.Requeued = tag.RowsAffected()
			return err
		}
		if err := exists(ctx, tx, endpointID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, requeue+` WHERE d.message_id = ANY ($2) AND d.endpoint_id = $1 AND d.status <> 'pending'`, endpointID, ids)
		if err != nil {
			return err
		}
		res.Requeued = tag.RowsAffected()
		tag, err = tx.Exec(ctx, create+`m.id = ANY ($2) ON CONFLICT (message_id, endpoint_id) DO NOTHING`, endpointID, ids)
		res.Created = tag.RowsAffected()
		return err
	})
	if err != nil {
		return Result{}, fmt.Errorf("replay: %w", err)
	}
	return res, nil
}

// Since catches an endpoint up on the messages created at or after since: failed
// deliveries are requeued, and the messages it never got are delivered to it. Its
// succeeded deliveries are not sent again.
func Since(ctx context.Context, conn *pgx.Conn, endpointID string, since time.Time) (Result, error) {
	var res Result
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if err := exists(ctx, tx, endpointID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, requeue+` FROM webhook_messages m
			WHERE m.id = d.message_id AND d.endpoint_id = $1 AND d.status = 'failed' AND m.created_at >= $2`, endpointID, since)
		if err != nil {
			return err
		}
		res.Requeued = tag.RowsAffected()
		tag, err = tx.Exec(ctx, create+`m.created_at >= $2 ON CONFLICT (message_id, endpoint_id) DO NOTHING`, endpointID, since)
		res.Created = tag.RowsAffected()
		return err
	})
	if err != nil {
		return Result{}, fmt.Errorf("replay: %w", err)
	}
	return res, nil
}

func exists(ctx context.Context, tx pgx.Tx, id string) error {
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM webhook_endpoints WHERE id = $1)`, id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", endpoint.ErrNotFound, id)
	}
	return nil
}

func missing(ctx context.Context, tx pgx.Tx, ids []string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT i FROM unnest($1::text[]) i
		WHERE NOT EXISTS (SELECT 1 FROM webhook_messages WHERE id = i) LIMIT 1`, ids).Scan(&id)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", ErrMessageNotFound, id)
}

func unique(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

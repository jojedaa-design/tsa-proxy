package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bigdavi/tsa-proxy/internal/model"
)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

func (r *AuditRepository) Insert(ctx context.Context, e *model.AuditEvent) error {
	var changesJSON []byte
	if e.Changes != nil {
		var err error
		changesJSON, err = json.Marshal(e.Changes)
		if err != nil {
			changesJSON = []byte("{}")
		}
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_events
		  (actor_id, actor_type, action, entity_type, entity_id, changes, ip_address, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, e.ActorID, e.ActorType, e.Action, e.EntityType, e.EntityID,
		changesJSON, e.IPAddress, e.UserAgent)
	return err
}

type AuditFilter struct {
	ActorID    *uuid.UUID
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	From       *time.Time
	To         *time.Time
	Page       int
	Limit      int
}

func (r *AuditRepository) List(ctx context.Context, f AuditFilter) ([]*model.AuditEvent, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 200 {
		f.Limit = 50
	}
	offset := (f.Page - 1) * f.Limit

	args := []interface{}{}
	where := "WHERE 1=1"
	argIdx := 1

	if f.ActorID != nil {
		where += fmt.Sprintf(" AND ae.actor_id=$%d", argIdx)
		args = append(args, *f.ActorID)
		argIdx++
	}
	if f.Action != "" {
		where += fmt.Sprintf(" AND ae.action ILIKE $%d", argIdx)
		args = append(args, "%"+f.Action+"%")
		argIdx++
	}
	if f.EntityType != "" {
		where += fmt.Sprintf(" AND ae.entity_type=$%d", argIdx)
		args = append(args, f.EntityType)
		argIdx++
	}
	if f.EntityID != nil {
		where += fmt.Sprintf(" AND ae.entity_id=$%d", argIdx)
		args = append(args, *f.EntityID)
		argIdx++
	}
	if f.From != nil {
		where += fmt.Sprintf(" AND ae.occurred_at >= $%d", argIdx)
		args = append(args, *f.From)
		argIdx++
	}
	if f.To != nil {
		where += fmt.Sprintf(" AND ae.occurred_at <= $%d", argIdx)
		args = append(args, *f.To)
		argIdx++
	}

	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM audit_events ae %s`, where)
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, offset)
	dataSQL := fmt.Sprintf(`
		SELECT ae.id, ae.actor_id, ae.actor_type, COALESCE(au.username,'system'),
		       ae.action, ae.entity_type, ae.entity_id,
		       ae.changes, ae.ip_address, ae.user_agent, ae.occurred_at
		FROM audit_events ae
		LEFT JOIN admin_users au ON au.id = ae.actor_id
		%s
		ORDER BY ae.occurred_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]*model.AuditEvent, 0)
	for rows.Next() {
		e := &model.AuditEvent{}
		var changesRaw []byte
		var ipStr, ua *string

		if err := rows.Scan(
			&e.ID, &e.ActorID, &e.ActorType, &e.ActorName,
			&e.Action, &e.EntityType, &e.EntityID,
			&changesRaw, &ipStr, &ua, &e.OccurredAt,
		); err != nil {
			return nil, 0, err
		}
		if changesRaw != nil {
			_ = json.Unmarshal(changesRaw, &e.Changes)
		}
		e.IPAddress = ipStr
		e.UserAgent = ua
		events = append(events, e)
	}
	return events, total, rows.Err()
}

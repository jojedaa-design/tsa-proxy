// Package jobs implementa tareas programadas de mantenimiento.
package jobs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// CleanupJob elimina registros más antiguos que 1 año.
// Dado el particionado mensual, hace DROP de la partición completa
// cuando corresponde, o DELETE clásico si no hay partición.
type CleanupJob struct {
	pool *pgxpool.Pool
}

func NewCleanupJob(pool *pgxpool.Pool) *CleanupJob {
	return &CleanupJob{pool: pool}
}

// Run ejecuta la limpieza. Se llama periódicamente (diario a las 03:00 UTC).
func (j *CleanupJob) Run(ctx context.Context) {
	cutoff := time.Now().AddDate(-1, 0, 0) // 1 año atrás

	log.Info().Time("cutoff", cutoff).Msg("starting data cleanup job")

	// Determinar cuáles particiones mensuales hay que eliminar
	// Las particiones tienen nombre: usage_events_YYYY_MM
	j.dropOldPartitions(ctx, "usage_events", cutoff)
	j.dropOldPartitions(ctx, "proxy_requests", cutoff)

	// Para failed_requests (no particionada), DELETE clásico
	j.deleteOldRows(ctx, "failed_requests", cutoff)
	j.deleteOldRows(ctx, "audit_events", cutoff)

	log.Info().Msg("data cleanup job completed")
}

func (j *CleanupJob) dropOldPartitions(ctx context.Context, tableName string, cutoff time.Time) {
	// Encontrar particiones más antiguas que el cutoff
	rows, err := j.pool.Query(ctx, `
		SELECT schemaname, tablename
		FROM pg_tables
		WHERE tablename LIKE $1
		  AND tablename < $2
		ORDER BY tablename
	`,
		tableName+"_%",
		tableName+"_"+cutoff.Format("2006_01"),
	)
	if err != nil {
		log.Error().Err(err).Str("table", tableName).Msg("error listing old partitions")
		return
	}
	defer rows.Close()

	type partition struct{ schema, name string }
	var partitions []partition
	for rows.Next() {
		var p partition
		if err := rows.Scan(&p.schema, &p.name); err == nil {
			partitions = append(partitions, p)
		}
	}

	for _, p := range partitions {
		if _, err := j.pool.Exec(ctx, "DROP TABLE IF EXISTS "+p.schema+"."+p.name); err != nil {
			log.Error().Err(err).Str("partition", p.name).Msg("error dropping partition")
		} else {
			log.Info().Str("partition", p.name).Msg("dropped old partition")
		}
	}
}

func (j *CleanupJob) deleteOldRows(ctx context.Context, tableName string, cutoff time.Time) {
	result, err := j.pool.Exec(ctx,
		"DELETE FROM "+tableName+" WHERE occurred_at < $1",
		cutoff,
	)
	if err != nil {
		log.Error().Err(err).Str("table", tableName).Msg("error deleting old rows")
		return
	}
	log.Info().
		Str("table", tableName).
		Int64("deleted", result.RowsAffected()).
		Msg("deleted old rows")
}

// PartitionJob crea particiones mensuales con anticipación.
type PartitionJob struct {
	pool *pgxpool.Pool
}

func NewPartitionJob(pool *pgxpool.Pool) *PartitionJob {
	return &PartitionJob{pool: pool}
}

// EnsureNextMonth asegura que exista la partición del mes siguiente.
// Se ejecuta el día 25 de cada mes.
func (j *PartitionJob) EnsureNextMonth(ctx context.Context) {
	// Crear partición para el mes siguiente
	next := time.Now().AddDate(0, 1, 0)
	start := time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	suffix := start.Format("2006_01")

	tables := []string{"usage_events", "proxy_requests"}
	for _, t := range tables {
		partName := t + "_" + suffix
		sql := "CREATE TABLE IF NOT EXISTS " + partName +
			" PARTITION OF " + t +
			" FOR VALUES FROM ('" + start.Format("2006-01-02") + "')" +
			" TO ('" + end.Format("2006-01-02") + "')"

		if _, err := j.pool.Exec(ctx, sql); err != nil {
			log.Error().Err(err).Str("partition", partName).Msg("error creating partition")
		} else {
			log.Info().Str("partition", partName).Msg("ensured partition exists")
		}
	}
}

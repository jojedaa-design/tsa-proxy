-- Umbral de alerta por bolsa (opcional: 1-99 %)
ALTER TABLE quota_bundles
  ADD COLUMN alert_threshold_percent INT CHECK (alert_threshold_percent BETWEEN 1 AND 99);

-- Registro de notificaciones enviadas (idempotencia: una notificación por bolsa × umbral)
CREATE TABLE bundle_notification_log (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  bundle_id         UUID        NOT NULL REFERENCES quota_bundles(id) ON DELETE CASCADE,
  threshold_percent INT         NOT NULL,
  brevo_message_id  TEXT,
  sent_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (bundle_id, threshold_percent)
);
CREATE INDEX idx_bnl_bundle ON bundle_notification_log(bundle_id);

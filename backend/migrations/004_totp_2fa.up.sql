-- Migración 004: Segundo factor de autenticación TOTP
ALTER TABLE admin_users
    ADD COLUMN IF NOT EXISTS totp_secret  TEXT    DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE;

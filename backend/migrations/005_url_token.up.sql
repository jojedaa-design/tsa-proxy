-- Agrega url_token a api_credentials para autenticación vía URL
-- (compatible con software de firma que no maneja auth headers correctamente)
ALTER TABLE api_credentials
    ADD COLUMN IF NOT EXISTS url_token VARCHAR(32) UNIQUE;

-- Backfill: generar token aleatorio para credenciales existentes
UPDATE api_credentials
SET url_token = encode(gen_random_bytes(16), 'hex')
WHERE url_token IS NULL;

-- Hacer NOT NULL después del backfill
ALTER TABLE api_credentials
    ALTER COLUMN url_token SET NOT NULL;

-- Índice para lookup rápido por url_token
CREATE UNIQUE INDEX IF NOT EXISTS idx_cred_url_token
    ON api_credentials(url_token)
    WHERE status = 'active';

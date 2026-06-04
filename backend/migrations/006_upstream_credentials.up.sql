-- Agrega credenciales directas al upstream (reemplaza el esquema de env_key_*)
-- El password se guarda en texto plano protegido por acceso a la BD
ALTER TABLE tsa_upstreams
    ADD COLUMN IF NOT EXISTS username TEXT DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS password TEXT DEFAULT NULL;

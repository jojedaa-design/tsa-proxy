// Package crypto cifra en reposo secretos que hoy vive en columnas de
// Postgres en texto plano (p.ej. la contraseña de un upstream TSA externo).
//
// Uso opcional y retrocompatible: si no se configura una clave, Encrypt
// devuelve el valor sin cambios (comportamiento previo) y Decrypt reconoce
// automáticamente valores en texto plano (sin el prefijo "enc:v1:") y los
// devuelve tal cual — así una fila cifrada y una sin cifrar conviven sin
// necesitar una migración de datos coordinada con el despliegue.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const prefix = "enc:v1:"

// KeySize es el tamaño requerido de la clave para AES-256-GCM.
const KeySize = 32

// ParseKey decodifica una clave de 32 bytes desde base64 (estándar).
// Devuelve (nil, nil) si raw está vacío — clave no configurada, no es un error.
func ParseKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("clave de cifrado inválida (no es base64 válido): %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("clave de cifrado debe tener %d bytes (tiene %d)", KeySize, len(key))
	}
	return key, nil
}

// Encrypt cifra plaintext con AES-256-GCM. Si key es nil (sin configurar),
// devuelve plaintext sin modificar.
func Encrypt(plaintext string, key []byte) (string, error) {
	if key == nil || plaintext == "" {
		return plaintext, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt descifra un valor producido por Encrypt. Si value no tiene el
// prefijo "enc:v1:" se asume texto plano legado y se devuelve sin cambios
// (permite convivencia con filas no migradas). Si key es nil pero el valor
// SÍ está cifrado, devuelve error explícito en vez de un secreto corrupto.
func Decrypt(value string, key []byte) (string, error) {
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	if key == nil {
		return "", errors.New("valor cifrado pero no hay clave de cifrado configurada (UPSTREAM_CREDENTIALS_KEY)")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", fmt.Errorf("valor cifrado corrupto: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("valor cifrado corrupto: demasiado corto")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("no se pudo descifrar (clave incorrecta o dato corrupto): %w", err)
	}
	return string(plaintext), nil
}

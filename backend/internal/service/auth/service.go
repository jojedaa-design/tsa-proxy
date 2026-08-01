package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/argon2"

	"github.com/bigdavi/tsa-proxy/internal/config"
	"github.com/bigdavi/tsa-proxy/internal/middleware"
	"github.com/bigdavi/tsa-proxy/internal/model"
	"github.com/bigdavi/tsa-proxy/internal/repository/postgres"
)

// Argon2id parameters — OWASP recommended minimum
const (
	argonMemory      = 64 * 1024 // 64 MB
	argonIterations  = 3
	argonParallelism = 4
	argonKeyLen      = 32
	argonSaltLen     = 16
)

// Service maneja autenticación administrativa.
type Service struct {
	cfg       *config.Config
	userRepo  *postgres.AdminUserRepository
	redisClient *redis.Client
}

func NewService(
	cfg *config.Config,
	userRepo *postgres.AdminUserRepository,
	redisClient *redis.Client,
) *Service {
	return &Service{
		cfg:         cfg,
		userRepo:    userRepo,
		redisClient: redisClient,
	}
}

// LoginResult se devuelve al login exitoso.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         *model.AdminUser
	// Si TOTPRequired=true, los tokens están vacíos y hay que continuar con VerifyTOTP
	TOTPRequired bool
	MFAToken     string
	// Si TOTPSetupRequired=true, el usuario aún no tiene 2FA activo (obligatorio) y
	// debe completar el setup con SetupTOTPWithToken + CompleteTOTPSetup antes de
	// obtener una sesión.
	TOTPSetupRequired bool
	SetupToken        string
}

// TOTPSetup contiene los datos para configurar 2FA.
type TOTPSetup struct {
	Secret  string
	QRCodeURL string // otpauth:// URL para QR
}

// Login valida credenciales y genera tokens.
// Si el usuario tiene 2FA activo, devuelve TOTPRequired=true y un MFAToken temporal.
func (s *Service) Login(ctx context.Context, username, password, ip string) (*LoginResult, error) {
	genericErr := fmt.Errorf("invalid credentials")

	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("login lookup: %w", err)
	}
	if user == nil {
		_ = verifyArgon2id("dummy_hash_to_prevent_timing", password)
		return nil, genericErr
	}

	if !user.IsActive {
		return nil, fmt.Errorf("account disabled")
	}

	if !verifyArgon2id(user.PasswordHash, password) {
		return nil, genericErr
	}

	// Si tiene 2FA habilitado, emitir MFA token temporal en vez de JWT
	if user.TOTPEnabled {
		mfaToken, err := s.generateMFAToken(ctx, user.ID)
		if err != nil {
			return nil, fmt.Errorf("generate mfa token: %w", err)
		}
		return &LoginResult{TOTPRequired: true, MFAToken: mfaToken}, nil
	}

	// 2FA es obligatorio: si aún no está activo, bloquear el acceso y forzar
	// el setup. No se emite ninguna sesión hasta completar CompleteTOTPSetup.
	setupToken, err := s.generateSetupToken(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("generate setup token: %w", err)
	}
	return &LoginResult{TOTPSetupRequired: true, SetupToken: setupToken}, nil
}

// VerifyTOTP valida el código TOTP con el MFA token temporal y emite JWT.
func (s *Service) VerifyTOTP(ctx context.Context, mfaToken, code string) (*LoginResult, error) {
	userIDStr, err := s.redisClient.Get(ctx, mfaTokenKey(mfaToken)).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("invalid or expired mfa token")
	}
	if err != nil {
		return nil, fmt.Errorf("mfa token lookup: %w", err)
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid mfa token")
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive {
		return nil, fmt.Errorf("user not found or inactive")
	}

	if user.TOTPSecret == nil {
		return nil, fmt.Errorf("totp not configured")
	}

	if !totp.Validate(code, *user.TOTPSecret) {
		return nil, fmt.Errorf("invalid totp code")
	}

	// Invalidar MFA token (uso único)
	_ = s.redisClient.Del(ctx, mfaTokenKey(mfaToken))

	return s.issueTokens(ctx, user, "")
}

// SetupTOTP genera un nuevo secreto TOTP y URL para el QR, sin activarlo aún.
func (s *Service) SetupTOTP(ctx context.Context, userID uuid.UUID, username string) (*TOTPSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "BIGDAVI",
		AccountName: username,
		Period:      30,
		Digits:      6,
	})
	if err != nil {
		return nil, fmt.Errorf("generate totp key: %w", err)
	}

	// Guardar secreto pendiente (aún no habilitado) en Redis con TTL 10 min
	pendingKey := "totp_pending:" + userID.String()
	if err := s.redisClient.Set(ctx, pendingKey, key.Secret(), 10*time.Minute).Err(); err != nil {
		return nil, fmt.Errorf("store pending totp: %w", err)
	}

	return &TOTPSetup{
		Secret:    key.Secret(),
		QRCodeURL: key.URL(),
	}, nil
}

// EnableTOTP verifica el primer código TOTP y activa 2FA para el usuario.
func (s *Service) EnableTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	pendingKey := "totp_pending:" + userID.String()
	secret, err := s.redisClient.Get(ctx, pendingKey).Result()
	if err == redis.Nil {
		return fmt.Errorf("no pending totp setup, start setup first")
	}
	if err != nil {
		return fmt.Errorf("pending totp lookup: %w", err)
	}

	if !totp.Validate(code, secret) {
		return fmt.Errorf("invalid totp code")
	}

	// Guardar secreto en BD y activar
	if err := s.userRepo.UpdateTOTP(ctx, userID, &secret, true); err != nil {
		return fmt.Errorf("save totp: %w", err)
	}

	_ = s.redisClient.Del(ctx, pendingKey)
	return nil
}

// AdminResetTOTP desactiva 2FA de otro usuario sin requerir su código actual
// (uso: el usuario perdió el dispositivo). El actor es un admin/superadmin, no
// el propio usuario, por eso no se valida ningún código. El usuario deberá
// reconfigurar 2FA obligatoriamente en su próximo login.
func (s *Service) AdminResetTOTP(ctx context.Context, targetUserID uuid.UUID) error {
	return s.userRepo.UpdateTOTP(ctx, targetUserID, nil, false)
}

// SetupTOTPWithToken resuelve el userID desde un setup_token de login (usuario
// sin 2FA aún) y genera un secreto TOTP pendiente — equivalente a SetupTOTP
// pero sin requerir JWT, para el flujo de setup obligatorio en el login.
func (s *Service) SetupTOTPWithToken(ctx context.Context, setupToken string) (*TOTPSetup, error) {
	userID, user, err := s.resolveSetupToken(ctx, setupToken)
	if err != nil {
		return nil, err
	}
	return s.SetupTOTP(ctx, userID, user.Username)
}

// CompleteTOTPSetup valida el código, activa 2FA permanentemente y emite JWT —
// combina EnableTOTP + issueTokens en un solo paso para el flujo de login
// obligatorio. Si el código es inválido, el setup_token NO se consume, para
// permitir reintentar sin volver a escanear el QR.
func (s *Service) CompleteTOTPSetup(ctx context.Context, setupToken, code string) (*LoginResult, error) {
	userID, user, err := s.resolveSetupToken(ctx, setupToken)
	if err != nil {
		return nil, err
	}

	if err := s.EnableTOTP(ctx, userID, code); err != nil {
		return nil, err
	}

	// Uso único, igual que mfa_token en VerifyTOTP.
	_ = s.redisClient.Del(ctx, setupTokenKey(setupToken))

	return s.issueTokens(ctx, user, "")
}

func (s *Service) resolveSetupToken(ctx context.Context, setupToken string) (uuid.UUID, *model.AdminUser, error) {
	userIDStr, err := s.redisClient.Get(ctx, setupTokenKey(setupToken)).Result()
	if err == redis.Nil {
		return uuid.Nil, nil, fmt.Errorf("invalid or expired setup token")
	}
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("setup token lookup: %w", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("invalid setup token")
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive {
		return uuid.Nil, nil, fmt.Errorf("user not found or inactive")
	}
	return userID, user, nil
}

// issueTokens genera access + refresh token para un usuario ya verificado.
func (s *Service) issueTokens(ctx context.Context, user *model.AdminUser, ip string) (*LoginResult, error) {
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.generateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if ip != "" {
		go func() {
			_ = s.userRepo.UpdateLoginInfo(context.Background(), user.ID, ip)
		}()
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.cfg.JWT.AccessTTL.Seconds()),
		User:         user,
	}, nil
}

// Refresh valida un refresh token y emite un nuevo access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (string, error) {
	key := refreshTokenKey(refreshToken)
	userIDStr, err := s.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired refresh token")
	}
	if err != nil {
		return "", fmt.Errorf("refresh token lookup: %w", err)
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid user id in refresh token")
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive {
		return "", fmt.Errorf("user not found or inactive")
	}

	return s.generateAccessToken(user)
}

// Logout invalida el refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.redisClient.Del(ctx, refreshTokenKey(refreshToken)).Err()
}

// HashPassword genera un hash Argon2id de la contraseña.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(password), salt,
		argonIterations, argonMemory, argonParallelism, argonKeyLen,
	)

	// Formato: $argon2id$v=19$m=65536,t=3,p=4$<salt_b64>$<hash_b64>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonIterations, argonParallelism, b64Salt, b64Hash), nil
}

// ─── Funciones privadas ──────────────────────────────────────

func (s *Service) generateAccessToken(user *model.AdminUser) (string, error) {
	now := time.Now()
	claims := middleware.AdminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWT.AccessTTL)),
			Issuer:    "tsa-proxy",
		},
		Username: user.Username,
		Roles:    user.Roles,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWT.Secret))
}

func (s *Service) generateMFAToken(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(b)
	if err := s.redisClient.Set(ctx, mfaTokenKey(token), userID.String(), 5*time.Minute).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) generateRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(b)

	key := refreshTokenKey(token)
	err := s.redisClient.Set(ctx, key, userID.String(), s.cfg.JWT.RefreshTTL).Err()
	if err != nil {
		return "", err
	}
	return token, nil
}

func refreshTokenKey(token string) string {
	return "refresh:" + token
}

func mfaTokenKey(token string) string {
	return "mfa:" + token
}

// generateSetupToken crea un token de un solo uso para el flujo de 2FA
// obligatorio en el login (usuario aún sin TOTP activo). TTL más largo que el
// mfaToken porque el usuario recién va a instalar/abrir la app autenticadora.
func (s *Service) generateSetupToken(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(b)
	if err := s.redisClient.Set(ctx, setupTokenKey(token), userID.String(), 15*time.Minute).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func setupTokenKey(token string) string {
	return "totp_setup:" + token
}

// verifyArgon2id verifica una contraseña contra un hash Argon2id almacenado.
func verifyArgon2id(hash, password string) bool {
	var memory, iterations uint32
	var parallelism uint8
	var b64Salt, b64Hash string

	_, err := fmt.Sscanf(hash,
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s",
		&memory, &iterations, &parallelism, &b64Salt)
	if err != nil {
		return false
	}

	// Separar salt y hash (están separados por $)
	parts := splitLast(b64Salt, "$")
	if len(parts) != 2 {
		return false
	}
	b64Salt = parts[0]
	b64Hash = parts[1]

	salt, err := base64.RawStdEncoding.DecodeString(b64Salt)
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(b64Hash)
	if err != nil {
		return false
	}

	actualHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))

	// Comparación en tiempo constante
	if len(actualHash) != len(expectedHash) {
		return false
	}
	var diff byte
	for i := range actualHash {
		diff |= actualHash[i] ^ expectedHash[i]
	}
	return diff == 0
}

func splitLast(s, sep string) []string {
	idx := -1
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

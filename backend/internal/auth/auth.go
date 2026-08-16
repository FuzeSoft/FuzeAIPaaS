
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

var ErrTokenRevoked = errors.New("token revoked")

func generateJTI() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}

const principalKey = "fuze_principal"

const DefaultTenantID = "default"

type Claims struct {
	UserID   string      `json:"sub"`
	Username string      `json:"name"`
	Role     models.Role `json:"role"`
	TenantID string      `json:"tid"`
	IssuedAt int64       `json:"iat"`
	Expires  int64       `json:"exp"`
	
	Scope string `json:"scope,omitempty"`
	
	JTI string `json:"jti,omitempty"`
}

const (
	ScopeMFA = "mfa" 
	ScopePAT = "pat" 
)

type UserStore interface {
	GetUserByUsername(username string) (*models.User, error)
	
	UpdateUser(u *models.User) error
	
	UpdateUserMFARecovery(userID, recoveryEnc string) error
	
	RecordLoginFailure(username string, maxFails, lockSec int) (*models.User, bool, error)
	
	ClearLoginFailures(username string) error
	
	GetUserByID(id string) (*models.User, error)
}

type Manager struct {
	secret []byte
	ttl    time.Duration

	mfa *MFAService
	
	mfaTTL time.Duration

	maxLoginFails int
	loginLockSec  int

	passkey *WebAuthnService

	tokenTouch func(ctx context.Context, jti string)

	revoked func(jti string) bool
	
	revokeAdd func(jti string)
}

func (m *Manager) Stop() {
	if m.mfa != nil {
		m.mfa.Stop()
	}
}

func (m *Manager) RevokeJTI(jti string) {
	if jti != "" && m.revokeAdd != nil {
		m.revokeAdd(jti)
	}
}

func (m *Manager) SetTokenTouch(fn func(ctx context.Context, jti string)) {
	m.tokenTouch = fn
}

func (m *Manager) SetRevokedCheck(fn func(jti string) bool) {
	m.revoked = fn
}

func (m *Manager) SetRevokeAdd(fn func(jti string)) {
	m.revokeAdd = fn
}

func (m *Manager) SetMFA(svc *MFAService) {
	m.mfa = svc
	m.mfaTTL = 5 * time.Minute
}

func (m *Manager) SetLoginLimit(maxFails, lockSec int) {
	m.maxLoginFails = maxFails
	m.loginLockSec = lockSec
}

func (m *Manager) SetPasskey(svc *WebAuthnService) {
	m.passkey = svc
}

func (m *Manager) PasskeyService() *WebAuthnService { return m.passkey }

func (m *Manager) ClearPasskeys(store UserStore, username string) error {
	u, err := store.GetUserByUsername(username)
	if err != nil || u == nil {
		return errors.New("user not found")
	}
	u.Passkeys = ""
	u.PasskeyEnabled = false
	return store.UpdateUser(u)
}

func NewManager() *Manager {
	secret := os.Getenv("AUTH_SECRET")
	if secret == "" {
		log.Println("[Auth] WARNING: AUTH_SECRET not set, using development default secret. This is insecure for production!")
		log.Println("[Auth] Set AUTH_SECRET environment variable with a strong random string (32+ chars) for production use.")
		secret = "fuze-dev-secret-change-me"
	}
	return &Manager{secret: []byte(secret), ttl: 24 * time.Hour}
}

var devSecrets = map[string]bool{
	"":                           true, 
	"fuze-dev-secret-change-me":  true,
	"dev-secret-please-override": true,
}

func NewManagerForEnv(authEnabled bool) *Manager {
	if !authEnabled {
		
		return NewManager()
	}
	secret := os.Getenv("AUTH_SECRET")
	if len(secret) < 32 {
		log.Fatalf("[Auth] FATAL: AUTH_ENABLED=true 但 AUTH_SECRET 缺失或过短(<32字符)。" +
			"使用公开/弱密钥签发令牌将导致任意身份伪造。请设置强随机 AUTH_SECRET 后重启。")
	}
	if devSecrets[secret] {
		log.Fatalf("[Auth] FATAL: AUTH_ENABLED=true 但 AUTH_SECRET 命中开发占位密钥 %q。"+
			"请改用生产环境独有的强随机密钥后重启。", secret)
	}
	return &Manager{secret: []byte(secret), ttl: 24 * time.Hour}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

var canonicalHeader = func() string {
	h, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	return b64(h)
}()

func (m *Manager) hmacSign(headerSeg, payloadSeg string) string {
	signingInput := headerSeg + "." + payloadSeg
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(signingInput))
	return b64(mac.Sum(nil))
}

func (m *Manager) sign(payload []byte) (string, error) {
	payloadSeg := b64(payload)
	return canonicalHeader + "." + payloadSeg + "." + m.hmacSign(canonicalHeader, payloadSeg), nil
}

func (m *Manager) Sign(c *Claims) (string, error) {
	now := time.Now()
	if c.IssuedAt == 0 {
		c.IssuedAt = now.Unix()
	}
	if c.Expires == 0 {
		c.Expires = now.Add(m.ttl).Unix()
	}
	
	if c.JTI == "" {
		c.JTI = generateJTI()
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return m.sign(payload)
}

func (m *Manager) Validate(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}
	
	if subtle.ConstantTimeCompare([]byte(parts[0]), []byte(canonicalHeader)) != 1 {
		return nil, errors.New("invalid token header")
	}
	
	expected := m.hmacSign(parts[0], parts[1])
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[2])) != 1 {
		return nil, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token payload")
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, errors.New("invalid token payload")
	}
	if c.Expires > 0 && time.Now().Unix() > c.Expires {
		return nil, errors.New("token expired")
	}
	
	if m.revoked != nil && c.JTI != "" && m.revoked(c.JTI) {
		return nil, ErrTokenRevoked
	}
	return &c, nil
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

var ErrAccountLocked = errors.New("account temporarily locked")

func (m *Manager) Login(store UserStore, username, password string) (string, *models.User, error) {
	u, err := store.GetUserByUsername(username)
	if err != nil || u == nil {
		return "", nil, errors.New("invalid credentials")
	}
	
	if m.maxLoginFails > 0 && u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return "", nil, ErrAccountLocked
	}
	if !u.Enabled {
		return "", nil, errors.New("user disabled")
	}
	if u.Password == "" {
		
		return "", nil, errors.New("use SSO login")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		
		if m.maxLoginFails > 0 {
			if _, locked, ferr := store.RecordLoginFailure(username, m.maxLoginFails, m.loginLockSec); ferr == nil {
				if locked {
					
					return "", nil, ErrAccountLocked
				}
			}
		}
		return "", nil, errors.New("invalid credentials")
	}
	
	if m.maxLoginFails > 0 {
		_ = store.ClearLoginFailures(username)
	}
	
	if u.MFARequired() || (m.passkey != nil && u.PasskeyEnabled) {
		token, err := m.signMFATempToken(u)
		if err != nil {
			return "", nil, err
		}
		return token, u, nil
	}
	token, err := m.Sign(&Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		TenantID: u.TenantID,
	})
	if err != nil {
		return "", nil, err
	}
	return token, u, nil
}

func (m *Manager) signMFATempToken(u *models.User) (string, error) {
	return m.Sign(&Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		TenantID: u.TenantID,
		Scope:    "mfa",
		
		Expires: time.Now().Add(m.mfaTTL).Unix(),
	})
}

var ErrInvalidMFACode = errors.New("invalid mfa code")

func (m *Manager) VerifyMFA(store UserStore, mfaToken, code string) (string, error) {
	if m.mfa == nil {
		return "", errors.New("mfa not configured")
	}
	claims, err := m.Validate(mfaToken)
	if err != nil {
		return "", errors.New("invalid or expired mfa token")
	}
	if claims.Scope != "mfa" {
		return "", errors.New("token is not an mfa challenge")
	}
	if time.Now().Unix() > claims.Expires {
		return "", errors.New("mfa token expired")
	}
	u, err := store.GetUserByUsername(claims.Username)
	if err != nil || u == nil {
		return "", errors.New("user not found")
	}
	
	if u.TOTPSecretEnc != "" {
		secret, derr := m.mfa.DecryptSecret(u.TOTPSecretEnc)
		if derr == nil && m.mfa.VerifyTOTP(secret, code, time.Now()) {
			return m.issueAfterMFA(u, claims.JTI)
		}
	}
	
	if u.MFARecoveryEnc != "" {
		newEnc, ok, rerr := m.mfa.ConsumeRecovery(u.MFARecoveryEnc, code)
		if rerr == nil && ok {
			if err := store.UpdateUserMFARecovery(u.ID, newEnc); err != nil {
				return "", errors.New("failed to persist recovery codes")
			}
			return m.issueAfterMFA(u, claims.JTI)
		}
	}
	
	if m.maxLoginFails > 0 {
		if _, locked, ferr := store.RecordLoginFailure(claims.Username, m.maxLoginFails, m.loginLockSec); ferr == nil && locked {
			return "", ErrAccountLocked
		}
	}
	return "", ErrInvalidMFACode
}

func (m *Manager) issueAfterMFA(u *models.User, mfaJTI string) (string, error) {
	token, err := m.Sign(&Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		TenantID: u.TenantID,
	})
	if err != nil {
		return "", err
	}
	
	if m.revokeAdd != nil && mfaJTI != "" {
		m.revokeAdd(mfaJTI)
	}
	return token, nil
}

func (m *Manager) VerifyPasskey(store UserStore, mfaToken string, r *http.Request) (string, error) {
	if m.passkey == nil {
		return "", errors.New("passkey not configured")
	}
	claims, err := m.Validate(mfaToken)
	if err != nil {
		return "", errors.New("invalid or expired mfa token")
	}
	if claims.Scope != "mfa" {
		return "", errors.New("token not a passkey challenge")
	}
	u, err := store.GetUserByID(claims.UserID)
	if err != nil || u == nil {
		return "", errors.New("user not found")
	}
	if !u.PasskeyEnabled {
		return "", ErrPasskeyNotConfigured
	}
	if err := m.passkey.FinishLogin(u, r); err != nil {
		return "", err
	}
	return m.issueAfterMFA(u, claims.JTI)
}

func (m *Manager) MFAService() *MFAService { return m.mfa }

func (m *Manager) BeginMFA(store UserStore, username, issuer string) (secret, uri string, recoveryCodes []string, err error) {
	if m.mfa == nil {
		return "", "", nil, errors.New("mfa not configured")
	}
	u, err := store.GetUserByUsername(username)
	if err != nil || u == nil {
		return "", "", nil, errors.New("user not found")
	}
	plainSecret, otpURI, gerr := m.mfa.GenerateSecret(username, issuer)
	if gerr != nil {
		return "", "", nil, gerr
	}
	encSecret, err := m.mfa.EncryptSecret(plainSecret)
	if err != nil {
		return "", "", nil, err
	}
	codes, encRecovery, err := m.mfa.GenerateRecovery(10)
	if err != nil {
		return "", "", nil, err
	}
	
	u.PendingTOTPEnc = encSecret
	u.MFARecoveryPendingEnc = encRecovery
	if err := store.UpdateUser(u); err != nil {
		return "", "", nil, errors.New("failed to persist mfa setup")
	}
	return plainSecret, otpURI, codes, nil
}

func (m *Manager) ConfirmMFA(store UserStore, username, code string) error {
	if m.mfa == nil {
		return errors.New("mfa not configured")
	}
	u, err := store.GetUserByUsername(username)
	if err != nil || u == nil {
		return errors.New("user not found")
	}
	if u.PendingTOTPEnc == "" {
		return errors.New("no pending mfa enrollment")
	}
	secret, derr := m.mfa.DecryptSecret(u.PendingTOTPEnc)
	if derr != nil {
		return errors.New("failed to decode pending secret")
	}
	if !m.mfa.VerifyTOTP(secret, code, time.Now()) {
		return ErrInvalidMFACode
	}
	u.TOTPSecretEnc = u.PendingTOTPEnc
	if u.MFARecoveryPendingEnc != "" {
		u.MFARecoveryEnc = u.MFARecoveryPendingEnc
	}
	u.PendingTOTPEnc = ""
	u.MFARecoveryPendingEnc = ""
	u.MFAEnabled = true
	if err := store.UpdateUser(u); err != nil {
		return errors.New("failed to finalize mfa setup")
	}
	return nil
}

func (m *Manager) DisableMFA(store UserStore, username string) error {
	u, err := store.GetUserByUsername(username)
	if err != nil || u == nil {
		return errors.New("user not found")
	}
	u.TOTPSecretEnc = ""
	u.MFARecoveryEnc = ""
	u.MFAEnabled = false
	return store.UpdateUser(u)
}

const cookieTokenName = "fuze_token"

func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		hdr := c.GetHeader("Authorization")
		raw := ""
		switch {
		case strings.HasPrefix(hdr, "Bearer "):
			raw = strings.TrimPrefix(hdr, "Bearer ")
		case strings.TrimSpace(hdr) != "":
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		default:
			
			raw, _ = c.Cookie(cookieTokenName)
		}
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := m.Validate(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		
		if claims.Scope == ScopeMFA {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "mfa challenge token cannot access resources; complete mfa first"})
			return
		}
		
		if m.tokenTouch != nil && claims.Scope == ScopePAT && claims.JTI != "" {
			go m.tokenTouch(c.Request.Context(), claims.JTI)
		}
		c.Set(principalKey, claims)
		c.Next()
	}
}

func (m *Manager) RequireRole(roles ...models.Role) gin.HandlerFunc {
	allow := make(map[models.Role]bool, len(roles))
	for _, r := range roles {
		allow[r] = true
	}
	return func(c *gin.Context) {
		claims, ok := Principal(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}
		if !allow[claims.Role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: role=" + string(claims.Role)})
			return
		}
		c.Next()
	}
}

func PassthroughMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(principalKey, &Claims{
			UserID:   "system",
			Username: "system-admin",
			Role:     models.RolePlatformAdmin,
			TenantID: "default",
		})
		c.Next()
	}
}

func Principal(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return nil, false
	}
	cl, ok := v.(*Claims)
	return cl, ok
}

func SetPrincipal(c *gin.Context, claims *Claims) {
	c.Set(principalKey, claims)
}

func SyntheticAdmin() *Claims {
	return &Claims{UserID: "system", Username: "system-admin", Role: models.RolePlatformAdmin, TenantID: DefaultTenantID}
}
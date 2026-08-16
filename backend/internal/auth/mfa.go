package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" 
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"fuze-ai-paas/backend/internal/crypto/aes"
)

type MFAService struct {
	cipher *aes.Cipher

	step   time.Duration 
	digits int           
	skew   int           

	usedMu  sync.Mutex
	used    map[string]time.Time
	usedTTL time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewMFAService(cipher *aes.Cipher) *MFAService {
	step := 30 * time.Second
	m := &MFAService{
		cipher:  cipher,
		step:    step,
		digits:  6,
		skew:    1,
		used:    make(map[string]time.Time),
		usedTTL: step * 3, 
		stopCh:  make(chan struct{}),
	}
	m.startCleanup()
	return m
}

func (m *MFAService) GenerateSecret(account, issuer string) (secretBase32, otpauthURI string, err error) {
	raw := make([]byte, 20) 
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("mfa: generate secret: %w", err)
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	
	uri := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&period=%d&digits=%d&algorithm=SHA1",
		urlEncode(issuer), urlEncode(account), secret, urlEncode(issuer), int(m.step.Seconds()), m.digits)
	return secret, uri, nil
}

func (m *MFAService) EncryptSecret(plainBase32 string) (string, error) {
	return m.cipher.EncryptString(plainBase32)
}
func (m *MFAService) DecryptSecret(enc string) (string, error) {
	return m.cipher.DecryptString(enc)
}

func (m *MFAService) counter(at time.Time) int64 {
	return at.Unix() / int64(m.step.Seconds())
}

func (m *MFAService) hotp(secretBytes []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, secretBytes)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := ((int(sum[offset]) & 0x7f) << 24) |
		((int(sum[offset+1]) & 0xff) << 16) |
		((int(sum[offset+2]) & 0xff) << 8) |
		(int(sum[offset+3]) & 0xff)
	mod := 1
	for i := 0; i < m.digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", m.digits, code%mod)
}

func (m *MFAService) computeTOTP(secretBase32 string, at time.Time) string {
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretBase32)
	if err != nil {
		
		if s2, e2 := base32.StdEncoding.DecodeString(secretBase32); e2 == nil {
			secret = s2
		} else {
			return ""
		}
	}
	return m.hotp(secret, m.counter(at))
}

func (m *MFAService) VerifyTOTP(secretBase32, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != m.digits {
		return false
	}
	c := m.counter(at)
	for d := -m.skew; d <= m.skew; d++ {
		cur := c + int64(d)
		expect := m.computeTOTPAtCounter(secretBase32, cur)
		if !hmac.Equal([]byte(expect), []byte(code)) {
			continue
		}
		
		if m.claimCounter(secretBase32, cur) {
			return true
		}
		return false
	}
	return false
}

func (m *MFAService) computeTOTPAtCounter(secretBase32 string, counter int64) string {
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretBase32)
	if err != nil {
		if s2, e2 := base32.StdEncoding.DecodeString(secretBase32); e2 == nil {
			secret = s2
		} else {
			return ""
		}
	}
	return m.hotp(secret, counter)
}

func (m *MFAService) usedKey(secret string, counter int64) string {
	sum := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("%x:%d", sum[:8], counter)
}

func (m *MFAService) claimCounter(secret string, counter int64) bool {
	m.usedMu.Lock()
	defer m.usedMu.Unlock()
	k := m.usedKey(secret, counter)
	if exp, ok := m.used[k]; ok && !time.Now().After(exp) {
		return false 
	}
	m.used[k] = time.Now().Add(m.usedTTL)
	return true
}

func (m *MFAService) startCleanup() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.usedTTL)
		defer ticker.Stop()
		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.sweepExpired()
			}
		}
	}()
}

func (m *MFAService) sweepExpired() {
	now := time.Now()
	m.usedMu.Lock()
	for k, exp := range m.used {
		if now.After(exp) {
			delete(m.used, k)
		}
	}
	m.usedMu.Unlock()
}

func (m *MFAService) Stop() {
	select {
	case <-m.stopCh:
		
	default:
		close(m.stopCh)
	}
	m.wg.Wait()
}

func (m *MFAService) GenerateRecovery(n int) (codes []string, enc string, err error) {
	codes = make([]string, 0, n)
	for i := 0; i < n; i++ {
		b := make([]byte, 10)
		if _, err := rand.Read(b); err != nil {
			return nil, "", fmt.Errorf("mfa: generate recovery: %w", err)
		}
		codes = append(codes, base64.RawURLEncoding.EncodeToString(b))
	}
	payload, err := json.Marshal(codes)
	if err != nil {
		return nil, "", err
	}
	enc, err = m.cipher.Encrypt(payload)
	if err != nil {
		return nil, "", err
	}
	return codes, enc, nil
}

func (m *MFAService) ConsumeRecovery(enc, code string) (newEnc string, ok bool, err error) {
	if enc == "" {
		return "", false, nil
	}
	plain, err := m.cipher.Decrypt(enc)
	if err != nil {
		return "", false, err
	}
	var codes []string
	if err := json.Unmarshal(plain, &codes); err != nil {
		return "", false, errors.New("mfa: corrupt recovery store")
	}
	idx := -1
	for i, c := range codes {
		if hmac.Equal([]byte(c), []byte(code)) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", false, nil
	}
	codes = append(codes[:idx], codes[idx+1:]...)
	if len(codes) == 0 {
		return "", true, nil
	}
	payload, err := json.Marshal(codes)
	if err != nil {
		return "", false, err
	}
	newEnc, err = m.cipher.Encrypt(payload)
	if err != nil {
		return "", false, err
	}
	return newEnc, true, nil
}

func urlEncode(s string) string {
	r := strings.NewReplacer(" ", "%20", "/", "%2F", ":", "%3A", "?", "%3F", "&", "%26", "=", "%3D")
	return r.Replace(s)
}
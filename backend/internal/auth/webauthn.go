package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"fuze-ai-paas/backend/internal/crypto/aes"
	"fuze-ai-paas/backend/internal/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var ErrPasskeyNotConfigured = errors.New("passkey not configured for user")

type WebAuthnUser struct {
	user        *models.User
	credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte                         { return []byte(u.user.ID) }
func (u *WebAuthnUser) WebAuthnName() string                       { return u.user.Username }
func (u *WebAuthnUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

type WebAuthnService struct {
	wa     *webauthn.WebAuthn
	cipher *aes.Cipher

	mu      sync.Mutex
	session map[string]*webauthn.SessionData 
}

func NewWebAuthnService(rpID, rpName string, rpOrigins []string, cipher *aes.Cipher) (*WebAuthnService, error) {
	if cipher == nil {
		return nil, errors.New("webauthn: cipher required for credential storage")
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     rpOrigins,
	})
	if err != nil {
		return nil, err
	}
	return &WebAuthnService{
		wa:      wa,
		cipher:  cipher,
		session: make(map[string]*webauthn.SessionData),
	}, nil
}

func (s *WebAuthnService) parseCredentials(u *models.User) ([]webauthn.Credential, error) {
	if u.Passkeys == "" {
		return nil, nil
	}
	plain, err := s.cipher.DecryptString(u.Passkeys)
	if err != nil {
		return nil, err
	}
	var creds []webauthn.Credential
	if err := json.Unmarshal([]byte(plain), &creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (s *WebAuthnService) serializeCredentials(creds []webauthn.Credential) (string, error) {
	b, err := json.Marshal(creds)
	if err != nil {
		return "", err
	}
	return s.cipher.EncryptString(string(b))
}

func (s *WebAuthnService) BeginRegistration(u *models.User) (*protocol.CredentialCreation, error) {
	creds, err := s.parseCredentials(u)
	if err != nil {
		return nil, err
	}
	wu := &WebAuthnUser{user: u, credentials: creds}
	creation, sessionData, err := s.wa.BeginRegistration(wu)
	if err != nil {
		return nil, err
	}
	s.putSession(string(u.ID), sessionData)
	return creation, nil
}

func (s *WebAuthnService) FinishRegistration(u *models.User, r *http.Request) (passkeysEnc string, enabled bool, err error) {
	sessionData, ok := s.takeSession(string(u.ID))
	if !ok {
		return "", false, errors.New("webauthn: no registration session (expired or missing)")
	}
	creds, err := s.parseCredentials(u)
	if err != nil {
		return "", false, err
	}
	wu := &WebAuthnUser{user: u, credentials: creds}
	cred, err := s.wa.FinishRegistration(wu, *sessionData, r)
	if err != nil {
		return "", false, err
	}
	creds = append(creds, *cred)
	enc, err := s.serializeCredentials(creds)
	if err != nil {
		return "", false, err
	}
	return enc, true, nil
}

func (s *WebAuthnService) BeginLogin(u *models.User) (*protocol.CredentialAssertion, error) {
	creds, err := s.parseCredentials(u)
	if err != nil {
		return nil, err
	}
	if len(creds) == 0 {
		return nil, ErrPasskeyNotConfigured
	}
	wu := &WebAuthnUser{user: u, credentials: creds}
	assertion, sessionData, err := s.wa.BeginLogin(wu)
	if err != nil {
		return nil, err
	}
	s.putSession(string(u.ID), sessionData)
	return assertion, nil
}

func (s *WebAuthnService) FinishLogin(u *models.User, r *http.Request) error {
	sessionData, ok := s.takeSession(string(u.ID))
	if !ok {
		return errors.New("webauthn: no login session (expired or missing)")
	}
	creds, err := s.parseCredentials(u)
	if err != nil {
		return err
	}
	wu := &WebAuthnUser{user: u, credentials: creds}
	if _, err := s.wa.FinishLogin(wu, *sessionData, r); err != nil {
		return err
	}
	return nil
}

func (s *WebAuthnService) putSession(userID string, sd *webauthn.SessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session[userID] = sd
	time.AfterFunc(5*time.Minute, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if cur, ok := s.session[userID]; ok && cur == sd {
			delete(s.session, userID)
		}
	})
}

func (s *WebAuthnService) takeSession(userID string) (*webauthn.SessionData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sd, ok := s.session[userID]
	if ok {
		delete(s.session, userID)
	}
	return sd, ok
}
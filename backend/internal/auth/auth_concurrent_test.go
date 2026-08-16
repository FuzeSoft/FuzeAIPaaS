package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func randID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func TestManagerConcurrentSignValidate(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager 返回 nil")
	}

	const workers = 16
	const iters = 50
	var wg sync.WaitGroup
	errCh := make(chan error, workers*iters)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				claims := &Claims{
					UserID:   randID(),
					Username: "u" + randID(),
					Role:     models.RoleDeveloper,
					JTI:      randID(),
				}
				tok, err := m.Sign(claims)
				if err != nil {
					errCh <- err
					continue
				}
				got, verr := m.Validate(tok)
				if verr != nil {
					errCh <- verr
					continue
				}
				if got.UserID != claims.UserID {
					errCh <- fmt.Errorf("user mismatch: got %s want %s", got.UserID, claims.UserID)
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("并发 Sign/Validate 失败: %v", err)
		}
	}
}

func TestManagerConcurrentRevokedCheck(t *testing.T) {
	m := NewManager()

	var mu sync.RWMutex
	blacklist := map[string]bool{}
	m.SetRevokedCheck(func(jti string) bool {
		mu.RLock()
		defer mu.RUnlock()
		return blacklist[jti]
	})
	m.SetRevokeAdd(func(jti string) {
		mu.Lock()
		blacklist[jti] = true
		mu.Unlock()
	})

	const workers = 16
	const iters = 50
	var wg sync.WaitGroup

	type pair struct {
		tok string
		jti string
	}
	tokens := make([]pair, workers*iters)
	for i := range tokens {
		jti := randID()
		tok, err := m.Sign(&Claims{UserID: "u", Username: "u", Role: models.RoleDeveloper, JTI: jti})
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		tokens[i] = pair{tok: tok, jti: jti}
		if i%3 == 0 {
			m.revokeAdd(jti)
		}
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for i := start; i < len(tokens); i += workers {
				_, err := m.Validate(tokens[i].tok)
				if tokens[i].jti != "" && i%3 == 0 {
					if err != ErrTokenRevoked {
						t.Errorf("已吊销令牌应返回 ErrTokenRevoked, got %v", err)
					}
				} else if err != nil {
					t.Errorf("未吊销令牌校验失败: %v", err)
				}
			}
		}(w)
	}
	wg.Wait()
}
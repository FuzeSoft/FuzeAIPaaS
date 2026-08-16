package auth

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

type LDAPConfig struct {
	Enabled       bool
	Addr          string 
	UseTLS        bool   
	SkipTLSVerify bool   
	UserDNFormat  string 
	DefaultRole   models.Role
	AdminGroups   []string
	AdminRole     models.Role
	DefaultTenant string
}

func LDAPLogin(cfg LDAPConfig, username, password string) (SSOUserInfo, error) {
	if !cfg.Enabled {
		return SSOUserInfo{}, errors.New("ldap: disabled")
	}
	if username == "" || password == "" {
		return SSOUserInfo{}, errors.New("ldap: username and password required")
	}
	if cfg.UserDNFormat == "" {
		return SSOUserInfo{}, errors.New("ldap: UserDNFormat not configured")
	}
	
	dn := fmt.Sprintf(cfg.UserDNFormat, escapeLDAPDNValue(username))
	if err := ldapBind(cfg.Addr, cfg.UseTLS, cfg.SkipTLSVerify, dn, password); err != nil {
		return SSOUserInfo{}, err
	}
	return SSOUserInfo{
		Provider:    "ldap",
		Subject:     username,
		Username:    username,
		DisplayName: username,
		Groups:      nil,
	}, nil
}

func escapeLDAPDNValue(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		switch {
		case r == ',' || r == '+' || r == '"' || r == '\\' || r == '<' || r == '>' || r == ';':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '#' && i == 0:
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == ' ' && (i == 0 || i == len(runes)-1):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ldapBind(addr string, useTLS, skipVerify bool, dn, password string) error {
	deadline := 8 * time.Second
	var conn net.Conn
	var err error
	if useTLS {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: deadline}, "tcp", addr, &tls.Config{InsecureSkipVerify: skipVerify})
	} else {
		conn, err = net.DialTimeout("tcp", addr, deadline)
	}
	if err != nil {
		return fmt.Errorf("ldap: connect %s failed: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(deadline)); err != nil {
		return fmt.Errorf("ldap: set deadline failed: %w", err)
	}

	version := tlv(0x02, []byte{0x03})  
	name := tlv(0x04, []byte(dn))       
	auth := tlv(0x80, []byte(password)) 
	bindSeq := tlv(0x30, append(append(version, name...), auth...))
	bindReq := tlv(0x61, bindSeq) 
	msg := tlv(0x30, append(berInt(1), bindReq...))

	if _, err := conn.Write(msg); err != nil {
		return fmt.Errorf("ldap: write bind request: %w", err)
	}
	resp := make([]byte, 8192)
	n, err := conn.Read(resp)
	if err != nil && n == 0 {
		return fmt.Errorf("ldap: read response: %w", err)
	}
	code, perr := parseBindResult(resp[:n])
	if perr != nil {
		return perr
	}
	if code != 0 {
		return fmt.Errorf("ldap bind failed for %s: resultCode=%d", dn, code)
	}
	return nil
}

func tlv(tag byte, content []byte) []byte {
	l := len(content)
	if l < 0x80 {
		return append([]byte{tag, byte(l)}, content...)
	}
	lengthBytes := []byte{}
	for l > 0 {
		lengthBytes = append([]byte{byte(l & 0xff)}, lengthBytes...)
		l >>= 8
	}
	return append([]byte{tag, 0x80 | byte(len(lengthBytes))}, append(lengthBytes, content...)...)
}

func berInt(n int) []byte {
	if n == 0 {
		return []byte{0x02, 0x01, 0x00}
	}
	b := []byte{}
	v := n
	for v > 0 {
		b = append([]byte{byte(v & 0xff)}, b...)
		v >>= 8
	}
	return append([]byte{0x02, byte(len(b))}, b...)
}

type berElem struct {
	tag  byte
	val  []byte
	rest []byte
}

func readTLV(b []byte) (berElem, error) {
	if len(b) < 2 {
		return berElem{}, errors.New("ldap: truncated TLV")
	}
	tag := b[0]
	l := int(b[1])
	off := 2
	if l&0x80 != 0 {
		num := l & 0x7f
		if num > 4 || len(b) < off+num {
			return berElem{}, errors.New("ldap: bad length")
		}
		l = 0
		for i := 0; i < num; i++ {
			l = l<<8 | int(b[off+i])
		}
		off += num
	}
	if len(b) < off+l {
		return berElem{}, errors.New("ldap: truncated value")
	}
	return berElem{tag: tag, val: b[off : off+l], rest: b[off+l:]}, nil
}

func parseBindResult(data []byte) (int, error) {
	msgElem, err := readTLV(data)
	if err != nil {
		return -1, fmt.Errorf("ldap: malformed message: %w", err)
	}
	idElem, err := readTLV(msgElem.val) 
	if err != nil {
		return -1, fmt.Errorf("ldap: malformed message: %w", err)
	}
	bindResp, err := readTLV(idElem.rest)
	if err != nil {
		return -1, fmt.Errorf("ldap: malformed response: %w", err)
	}
	if bindResp.tag != 0x61 {
		return -1, fmt.Errorf("ldap: unexpected response tag 0x%x", bindResp.tag)
	}
	rc, err := readTLV(bindResp.val)
	if err != nil {
		return -1, fmt.Errorf("ldap: no result code: %w", err)
	}
	if rc.tag != 0x0a {
		return -1, errors.New("ldap: result code element missing")
	}
	if len(rc.val) == 0 {
		return -1, errors.New("ldap: empty result code")
	}
	return int(rc.val[0]), nil
}
package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type sessionPayload struct {
	User string `json:"user"`
	CSRF string `json:"csrf"`
	Exp  int64  `json:"exp"`
}

func CheckPassword(given, want string) bool {
	givenHash := sha256.Sum256([]byte(given))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(givenHash[:], wantHash[:]) == 1
}

func SignSession(secret, user, csrf string, exp time.Time) (string, error) {
	payload := sessionPayload{
		User: user,
		CSRF: csrf,
		Exp:  exp.Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(encoded)); err != nil {
		return "", err
	}
	sig := hex.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, nil
}

func VerifySession(secret, cookie string, now time.Time) (user, csrf string, err error) {
	payloadPart, sigPart, ok := strings.Cut(cookie, ".")
	if !ok || payloadPart == "" || sigPart == "" {
		return "", "", errors.New("invalid session cookie")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(payloadPart)); err != nil {
		return "", "", err
	}
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sigPart)) {
		return "", "", errors.New("invalid session signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return "", "", fmt.Errorf("decode session payload: %w", err)
	}
	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", fmt.Errorf("parse session payload: %w", err)
	}
	if !time.Unix(p.Exp, 0).After(now) {
		return "", "", errors.New("session expired")
	}
	return p.User, p.CSRF, nil
}

func NewCSRF() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

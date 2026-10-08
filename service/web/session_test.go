package web

import (
	"testing"
	"time"
)

func TestCheckPassword(t *testing.T) {
	if !CheckPassword("admin-pass", "admin-pass") {
		t.Fatal("相同密码应通过")
	}
	if CheckPassword("admin-pass", "other-pass") {
		t.Fatal("不同密码应失败")
	}
}

func TestSignAndVerifySession(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	cookie, err := SignSession("session-secret-must-be-32-characters-min", "admin", "csrf-token", exp)
	if err != nil {
		t.Fatal(err)
	}
	user, csrf, err := VerifySession("session-secret-must-be-32-characters-min", cookie, time.Now())
	if err != nil || user != "admin" || csrf != "csrf-token" {
		t.Fatalf("got %s %s %v", user, csrf, err)
	}
	if _, _, err := VerifySession("session-secret-must-be-32-characters-min", cookie, exp.Add(time.Second)); err == nil {
		t.Fatal("过期会话应失败")
	}
	if _, _, err := VerifySession("session-secret-must-be-32-characters-min", "x"+cookie, time.Now()); err == nil {
		t.Fatal("篡改会话应失败")
	}
}

func TestNewCSRF(t *testing.T) {
	a, err := NewCSRF()
	if err != nil || a == "" {
		t.Fatal(err)
	}
	b, err := NewCSRF()
	if err != nil || a == b {
		t.Fatalf("两次 csrf 不应相同: %s %s", a, b)
	}
}

package sync

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
)

func TestAuthenticatorAnonymous(t *testing.T) {
	if authenticator(nil) != authn.Anonymous {
		t.Fatal("nil 认证应为匿名")
	}
	if authenticator(&Auth{}) != authn.Anonymous {
		t.Fatal("空用户名应为匿名")
	}
}

func TestAuthenticatorBasic(t *testing.T) {
	got := authenticator(&Auth{Username: "dest-user", Password: "registry-pass"})
	basic, ok := got.(*authn.Basic)
	if !ok || basic.Username != "dest-user" || basic.Password != "registry-pass" {
		t.Fatalf("got %#v", got)
	}
}

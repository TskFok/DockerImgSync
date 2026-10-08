package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte("a"), 32)
	encoded, err := Encrypt(key, "registry-pass")
	if err != nil {
		t.Fatal(err)
	}
	if encoded == "registry-pass" || encoded == "" {
		t.Fatalf("密文不应等于原文: %s", encoded)
	}
	got, err := Decrypt(key, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got != "registry-pass" {
		t.Fatalf("解密 = %s", got)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	key := bytes.Repeat([]byte("a"), 32)
	encoded, err := Encrypt(key, "registry-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(bytes.Repeat([]byte("b"), 32), encoded); err == nil {
		t.Fatal("错误密钥应解密失败")
	}
}

func TestEncryptRejectsBadKeyLength(t *testing.T) {
	if _, err := Encrypt([]byte("short"), "x"); err == nil {
		t.Fatal("密钥长度不对时应失败")
	}
}

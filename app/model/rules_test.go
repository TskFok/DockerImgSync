package model

import "testing"

func TestCanDeleteCredential(t *testing.T) {
	if err := CanDeleteCredential(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := CanDeleteCredential(1, 0); err != ErrInUse {
		t.Fatalf("got %v", err)
	}
	if err := CanDeleteCredential(0, 2); err != ErrInUse {
		t.Fatalf("got %v", err)
	}
}

func TestCanDeleteRegistry(t *testing.T) {
	if err := CanDeleteRegistry(0); err != nil {
		t.Fatal(err)
	}
	if err := CanDeleteRegistry(1); err != ErrInUse {
		t.Fatalf("got %v", err)
	}
}

func TestCanDeleteTask(t *testing.T) {
	if err := CanDeleteTask("success"); err != nil {
		t.Fatal(err)
	}
	if err := CanDeleteTask("running"); err != ErrTaskRunning {
		t.Fatalf("got %v", err)
	}
}

func TestValidateAddress(t *testing.T) {
	if err := ValidateAddress("registry.cn-hangzhou.aliyuncs.com"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAddress("127.0.0.1:5000"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "https://registry.example.com", "registry.example.com/v2"} {
		if err := ValidateAddress(bad); err == nil || err.Error() != "地址不能带协议或路径" {
			t.Fatalf("%q got %v", bad, err)
		}
	}
}

func TestValidateNamespace(t *testing.T) {
	if err := ValidateNamespace("myns"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "a/b"} {
		if err := ValidateNamespace(bad); err == nil || err.Error() != "命名空间不能含 /" {
			t.Fatalf("%q got %v", bad, err)
		}
	}
}

func TestValidateInterval(t *testing.T) {
	if err := ValidateInterval(0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInterval(60); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []int{-1, 1, 59} {
		if err := ValidateInterval(bad); err == nil || err.Error() != "检查间隔至少 60 秒" {
			t.Fatalf("%d got %v", bad, err)
		}
	}
}

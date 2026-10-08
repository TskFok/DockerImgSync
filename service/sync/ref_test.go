package sync

import "testing"

func TestParseSourceOfficialImage(t *testing.T) {
	normalized, repo, tag, err := ParseSource("nginx:latest")
	if err != nil {
		t.Fatal(err)
	}
	if normalized != "docker.io/library/nginx:latest" || repo != "nginx" || tag != "latest" {
		t.Fatalf("got %s %s %s", normalized, repo, tag)
	}
}

func TestParseSourceNestedPathUsesLastSegment(t *testing.T) {
	_, repo, tag, err := ParseSource("ghcr.io/org/team/app:1")
	if err != nil {
		t.Fatal(err)
	}
	if repo != "app" || tag != "1" {
		t.Fatalf("got %s %s", repo, tag)
	}
}

func TestParseSourceRejectsDigestOnly(t *testing.T) {
	_, _, _, err := ParseSource("nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || err.Error() != "源镜像必须包含 tag" {
		t.Fatalf("got %v", err)
	}
}

func TestDestRefUsesOverride(t *testing.T) {
	got, err := DestRef("registry.cn-hangzhou.aliyuncs.com", "myns", "nginx", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if got != "registry.cn-hangzhou.aliyuncs.com/myns/nginx:latest" {
		t.Fatalf("got %s", got)
	}
}

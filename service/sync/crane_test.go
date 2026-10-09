package sync

import (
	"encoding/json"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
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

func TestWithoutAttestationsDropsAnnotatedChildren(t *testing.T) {
	image := v1.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Size:      2190,
		Digest:    mustHash(t, "sha256:daff148274c7d28548185931ff9a49e338c478ef633f0976e1c8bbe0ba6861a2"),
		Platform:  &v1.Platform{OS: "linux", Architecture: "amd64"},
	}
	attestation := v1.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Size:      1112,
		Digest:    mustHash(t, "sha256:fb16e77db25b300c89dad824934db669056184575d0ceeb03b3804fdb6cc98c7"),
		Annotations: map[string]string{
			"vnd.docker.reference.type":   "attestation-manifest",
			"vnd.docker.reference.digest": image.Digest.String(),
		},
		Platform: &v1.Platform{OS: "unknown", Architecture: "unknown"},
	}
	desc := indexDescriptor(t, types.OCIImageIndex, image, attestation)

	got, err := withoutAttestations(desc)
	if err != nil {
		t.Fatal(err)
	}
	manifests := indexManifests(t, got)
	if len(manifests) != 1 || manifests[0].Digest != image.Digest {
		t.Fatalf("保留的清单 = %+v", manifests)
	}
	if manifests[0].Platform == nil || manifests[0].Platform.Architecture != "amd64" {
		t.Fatalf("平台 = %+v", manifests[0].Platform)
	}
}

func TestWithoutAttestationsKeepsIndexBytes(t *testing.T) {
	image := v1.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Size:      2190,
		Digest:    mustHash(t, "sha256:daff148274c7d28548185931ff9a49e338c478ef633f0976e1c8bbe0ba6861a2"),
	}
	desc := indexDescriptor(t, types.DockerManifestList, image)
	original := append([]byte(nil), desc.Manifest...)

	got, err := withoutAttestations(desc)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := got.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(original) {
		t.Fatalf("清单被改写: %s", raw)
	}
}

func TestWithoutAttestationsLeavesImage(t *testing.T) {
	raw := []byte(`{"schemaVersion":2}`)
	desc := &remote.Descriptor{
		Descriptor: v1.Descriptor{MediaType: types.OCIManifestSchema1},
		Manifest:   raw,
	}
	got, err := withoutAttestations(desc)
	if err != nil {
		t.Fatal(err)
	}
	if got != desc {
		t.Fatal("单架构镜像应原样返回")
	}
}

func TestWithoutAttestationsErrorsWhenNoneRemain(t *testing.T) {
	attestation := v1.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Size:      1112,
		Digest:    mustHash(t, "sha256:fb16e77db25b300c89dad824934db669056184575d0ceeb03b3804fdb6cc98c7"),
		Annotations: map[string]string{
			"vnd.docker.reference.type": "attestation-manifest",
		},
	}
	desc := indexDescriptor(t, types.OCIImageIndex, attestation)
	if _, err := withoutAttestations(desc); err == nil {
		t.Fatal("只剩 attestation 时应失败")
	}
}

func TestWithoutAttestationsRejectsInvalidIndex(t *testing.T) {
	desc := &remote.Descriptor{
		Descriptor: v1.Descriptor{MediaType: types.OCIImageIndex},
		Manifest:   []byte(`{`),
	}
	if _, err := withoutAttestations(desc); err == nil {
		t.Fatal("非法索引应失败")
	}
}

func indexDescriptor(t *testing.T, mediaType types.MediaType, manifests ...v1.Descriptor) *remote.Descriptor {
	t.Helper()
	raw, err := json.Marshal(v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     mediaType,
		Manifests:     manifests,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &remote.Descriptor{
		Descriptor: v1.Descriptor{MediaType: mediaType},
		Manifest:   raw,
	}
}

func indexManifests(t *testing.T, taggable remote.Taggable) []v1.Descriptor {
	t.Helper()
	raw, err := taggable.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	var idx v1.IndexManifest
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	return idx.Manifests
}

func mustHash(t *testing.T, digest string) v1.Hash {
	t.Helper()
	hash, err := v1.NewHash(digest)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

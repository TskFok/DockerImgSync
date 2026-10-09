package sync

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	attestationTypeAnnotation = "vnd.docker.reference.type"
	attestationManifest       = "attestation-manifest"
)

type Crane struct{}

func NewCrane() *Crane {
	return &Crane{}
}

func authenticator(auth *Auth) authn.Authenticator {
	if auth == nil || auth.Username == "" {
		return authn.Anonymous
	}
	return &authn.Basic{Username: auth.Username, Password: auth.Password}
}

func (c *Crane) Digest(ctx context.Context, ref string, auth *Auth) (string, error) {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return "", err
	}
	puller, err := remote.NewPuller(remote.WithAuth(authenticator(auth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return "", err
	}
	desc, err := puller.Head(ctx, parsed)
	if err != nil {
		return "", err
	}
	return desc.Digest.String(), nil
}

func withoutAttestations(desc *remote.Descriptor) (remote.Taggable, error) {
	if desc == nil || !desc.MediaType.IsIndex() {
		return desc, nil
	}
	idx, err := desc.ImageIndex()
	if err != nil {
		return nil, err
	}
	original, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("解析镜像索引失败: %w", err)
	}
	filtered := mutate.RemoveManifests(idx, match.Annotation(attestationTypeAnnotation, attestationManifest))
	next, err := filtered.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("解析镜像索引失败: %w", err)
	}
	if len(next.Manifests) == len(original.Manifests) {
		return desc, nil
	}
	if len(next.Manifests) == 0 {
		return nil, errors.New("索引中没有可复制的镜像清单")
	}
	return filtered, nil
}

func (c *Crane) Copy(ctx context.Context, src, dst string, srcAuth, dstAuth *Auth) error {
	srcRef, err := name.ParseReference(src)
	if err != nil {
		return err
	}
	dstRef, err := name.ParseReference(dst)
	if err != nil {
		return err
	}
	puller, err := remote.NewPuller(remote.WithAuth(authenticator(srcAuth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return err
	}
	desc, err := puller.Get(ctx, srcRef)
	if err != nil {
		return err
	}
	taggable, err := withoutAttestations(desc)
	if err != nil {
		return err
	}
	pusher, err := remote.NewPusher(remote.WithAuth(authenticator(dstAuth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return err
	}
	return pusher.Push(ctx, dstRef, taggable)
}

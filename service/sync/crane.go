package sync

import (
	"context"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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
	pusher, err := remote.NewPusher(remote.WithAuth(authenticator(dstAuth)), remote.WithTransport(http.DefaultTransport))
	if err != nil {
		return err
	}
	return pusher.Push(ctx, dstRef, desc)
}

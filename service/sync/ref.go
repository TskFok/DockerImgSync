package sync

import (
	"errors"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

// ParseSource parses a source image reference into normalized form, repository name, and tag.
func ParseSource(image string) (normalized, repository, tag string, err error) {
	ref, err := name.ParseReference(image, name.WeakValidation)
	if err != nil {
		return "", "", "", err
	}

	tagRef, ok := ref.(name.Tag)
	if !ok {
		return "", "", "", errors.New("源镜像必须包含 tag")
	}

	normalized = ref.Name()
	normalized = strings.Replace(normalized, "index.docker.io", "docker.io", 1)

	repoPath := ref.Context().RepositoryStr()
	parts := strings.Split(repoPath, "/")
	repository = parts[len(parts)-1]
	tag = tagRef.TagStr()

	return normalized, repository, tag, nil
}

// DestRef builds a destination image reference from registry address, namespace, repository, and tag.
func DestRef(address, namespace, repository, tag string) (string, error) {
	if address == "" || namespace == "" || repository == "" || tag == "" {
		return "", errors.New("destination reference requires address, namespace, repository, and tag")
	}
	return address + "/" + namespace + "/" + repository + ":" + tag, nil
}

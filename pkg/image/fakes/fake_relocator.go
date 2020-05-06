package fakes

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Relocator struct {
}

func (r *Relocator) Relocate(image v1.Image, dest string) (string, error) {
	digest, err := image.Digest()
	if err != nil {
		return "", err
	}
	sha := digest.String()

	destRef, err := name.ParseReference(dest)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s@%s", destRef.Context().RegistryStr(), destRef.Context().RepositoryStr(), sha), nil
}
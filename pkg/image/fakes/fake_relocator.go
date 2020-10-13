// Copyright 2020-Present VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package fakes

import (
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/pivotal/build-service-cli/pkg/registry"
)

type Relocator struct {
	callCount int
}

func (r *Relocator) Relocate(_ io.Writer, image v1.Image, dest string, _ registry.TLSConfig) (string, error) {
	r.callCount++
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

func (r *Relocator) CallCount() int {
	return r.callCount
}

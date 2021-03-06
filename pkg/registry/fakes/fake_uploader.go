// Copyright 2020-Present VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package fakes

import (
	"io"

	"github.com/pivotal/build-service-cli/pkg/registry"
)

type SourceUploader struct {
	ImageRef string
}

func (f *SourceUploader) Upload(_, _ string, _ io.Writer, _ registry.TLSConfig) (string, error) {
	return f.ImageRef, nil
}

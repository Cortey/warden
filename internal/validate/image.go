/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validate

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	tagDelim = ":"
)

//go:generate mockery --name=ImageValidatorService
type ImageValidatorService interface {
	Validate(ctx context.Context, image string) error
}

type ServiceConfig struct {
	NotaryConfig      NotaryConfig
	AllowedRegistries []string
}

type notaryService struct {
	ServiceConfig
	RepoFactory RepoFactory
}

func NewImageValidator(sc *ServiceConfig, notaryClientFactory RepoFactory) ImageValidatorService {
	return &notaryService{
		ServiceConfig: ServiceConfig{
			NotaryConfig:      sc.NotaryConfig,
			AllowedRegistries: sc.AllowedRegistries,
		},
		RepoFactory: notaryClientFactory,
	}
}

func (s *notaryService) Validate(ctx context.Context, image string) error {

	split := strings.Split(image, tagDelim)

	if len(split) != 2 {
		return errors.New("image name is not formatted correctly")
	}

	imgRepo := split[0]
	imgTag := split[1]

	if allowed := s.isImageAllowed(imgRepo); allowed {
		return nil
	}

	expectedShaBytes, err := s.getNotaryImageDigestHash(ctx, imgRepo, imgTag)
	if err != nil {
		return err
	}

	shaBytes, err := s.getImageDigestHash(image)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare(shaBytes, expectedShaBytes) == 0 {
		return errors.New("unexpected image hash value")
	}

	return nil
}

func (s *notaryService) isImageAllowed(imgRepo string) bool {
	for _, allowed := range s.AllowedRegistries {
		// repository is in allowed list
		if strings.HasPrefix(imgRepo, allowed) {
			return true
		}
	}
	return false
}

func (s *notaryService) getImageDigestHash(image string) ([]byte, error) {
	if len(image) == 0 {
		return []byte{}, errors.New("empty image provided")
	}

	ref, err := name.ParseReference(image)
	if err != nil {
		return []byte{}, fmt.Errorf("ref parse: %w", err)
	}
	i, err := remote.Image(ref)
	if err != nil {
		return []byte{}, fmt.Errorf("get image: %w", err)
	}
	m, err := i.Manifest()
	if err != nil {
		return []byte{}, fmt.Errorf("image manifest: %w", err)
	}

	bytes, err := hex.DecodeString(m.Config.Digest.Hex)

	if err != nil {
		return []byte{}, fmt.Errorf("checksum error: %w", err)
	}

	return bytes, nil
}

func (s *notaryService) getNotaryImageDigestHash(ctx context.Context, imgRepo, imgTag string) ([]byte, error) {
	if len(imgRepo) == 0 || len(imgTag) == 0 {
		return []byte{}, errors.New("empty arguments provided")
	}

	c, err := s.RepoFactory.NewRepoClient(imgRepo, s.NotaryConfig)
	if err != nil {
		return []byte{}, err
	}

	target, err := c.GetTargetByName(imgTag)
	if err != nil {
		return []byte{}, err
	}

	if len(target.Hashes) == 0 {
		return []byte{}, errors.New("image hash is missing")
	}

	if len(target.Hashes) > 1 {
		return []byte{}, errors.New("more than one hash for image")
	}

	key := ""
	for i := range target.Hashes {
		key = i
	}

	return target.Hashes[key], nil
}

/*
Copyright 2019 The Skaffold Authors

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

package cache

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/constants"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/docker"
	runcontext "github.com/GoogleContainerTools/skaffold/pkg/skaffold/runner/context"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/schema/latest"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/util"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// For testing
var (
	newDockerClient = createDockerClient
)

// ImageDetails holds the Digest and ID of an image
type ImageDetails struct {
	Digest string `yaml:"digest,omitempty"`
	ID     string `yaml:"id,omitempty"`
}

// ArtifactCache is a map of [artifact dependencies hash : ImageDetails]
type ArtifactCache map[string]ImageDetails

// cache holds any data necessary for accessing the cache
type cache struct {
	artifactCache      ArtifactCache
	dependencies       DependencyLister
	client             docker.LocalDaemon
	insecureRegistries map[string]bool
	cacheFile          string
	imagesAreLocal     bool
}

// DependencyLister fetches a list of dependencies for an artifact
type DependencyLister interface {
	DependenciesForArtifact(ctx context.Context, artifact *latest.Artifact) ([]string, error)
}

// NewCache returns the current state of the cache
func NewCache(runCtx *runcontext.RunContext, imagesAreLocal bool, dependencies DependencyLister) (Cache, error) {
	if !runCtx.Opts.CacheArtifacts {
		return &noCache{}, nil
	}

	cacheFile, err := resolveCacheFile(runCtx.Opts.CacheFile)
	if err != nil {
		logrus.Warnf("Error resolving cache file, not using skaffold cache: %v", err)
		return &noCache{}, nil
	}

	artifactCache, err := retrieveArtifactCache(cacheFile)
	if err != nil {
		logrus.Warnf("Error retrieving artifact cache, not using skaffold cache: %v", err)
		return &noCache{}, nil
	}

	client, err := newDockerClient(runCtx)
	if imagesAreLocal && err != nil {
		return nil, errors.Wrap(err, "getting local Docker client")
	}

	return &cache{
		artifactCache:      artifactCache,
		dependencies:       dependencies,
		client:             client,
		insecureRegistries: runCtx.InsecureRegistries,
		cacheFile:          cacheFile,
		imagesAreLocal:     imagesAreLocal,
	}, nil
}

func createDockerClient(runCtx *runcontext.RunContext) (docker.LocalDaemon, error) {
	return docker.NewAPIClient(runCtx.Opts.Prune(), runCtx.InsecureRegistries)
}

// resolveCacheFile makes sure that either a passed in cache file or the default cache file exists
func resolveCacheFile(cacheFile string) (string, error) {
	if cacheFile != "" {
		return cacheFile, util.VerifyOrCreateFile(cacheFile)
	}
	home, err := homedir.Dir()
	if err != nil {
		return "", errors.Wrap(err, "retrieving home directory")
	}
	defaultFile := filepath.Join(home, constants.DefaultSkaffoldDir, constants.DefaultCacheFile)
	return defaultFile, util.VerifyOrCreateFile(defaultFile)
}

func retrieveArtifactCache(cacheFile string) (ArtifactCache, error) {
	cache := ArtifactCache{}
	contents, err := ioutil.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(contents, &cache); err != nil {
		return nil, err
	}
	return cache, nil
}

func saveArtifactCache(cacheFile string, contents ArtifactCache) error {
	data, err := yaml.Marshal(contents)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(cacheFile, data, 0755)
}

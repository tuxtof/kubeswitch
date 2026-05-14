// Copyright 2021 The Kubeswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/karrick/godirwalk"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	storetypes "github.com/danielfoehrkn/kubeswitch/pkg/store/types"
	"github.com/danielfoehrkn/kubeswitch/types"
)

func NewFilesystemStore(
	kubeconfigName string,
	kubeconfigStore types.KubeconfigStore,
) (*FilesystemStore, error) {
	filesystemStoreConfig := &types.StoreConfigFilesystem{}
	if kubeconfigStore.Config != nil {
		buf, err := yaml.Marshal(kubeconfigStore.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal filesystem store config: %w", err)
		}
		if err := yaml.Unmarshal(buf, filesystemStoreConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal filesystem store config: %w", err)
		}
	}

	disableDirectorySearch := false
	if filesystemStoreConfig.DisableDirectorySearch != nil {
		disableDirectorySearch = *filesystemStoreConfig.DisableDirectorySearch
	}

	return &FilesystemStore{
		Logger:                 logrus.New().WithField("store", types.StoreKindFilesystem),
		KubeconfigStore:        kubeconfigStore,
		KubeconfigName:         kubeconfigName,
		DisableDirectorySearch: disableDirectorySearch,
	}, nil
}

func (s *FilesystemStore) GetContextPrefix(path string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}

	// return the name of the parent directory
	return filepath.Base(filepath.Dir(path))
}

func (s *FilesystemStore) GetID() string {
	id := "default"
	if s.KubeconfigStore.ID != nil {
		id = *s.KubeconfigStore.ID
	}
	return fmt.Sprintf("%s.%s", types.StoreKindFilesystem, id)
}

func (s *FilesystemStore) GetStoreConfig() types.KubeconfigStore {
	return s.KubeconfigStore
}

func (s *FilesystemStore) GetKind() types.StoreKind {
	return types.StoreKindFilesystem
}

func (s *FilesystemStore) GetLogger() *logrus.Entry {
	return s.Logger
}

func (s *FilesystemStore) StartSearch(channel chan storetypes.SearchResult) {
	for _, path := range s.kubeconfigFilepaths {
		channel <- storetypes.SearchResult{
			KubeconfigPath: path,
			Error:          nil,
		}
	}

	wg := sync.WaitGroup{}
	for _, path := range s.kubeconfigDirectories {
		wg.Add(1)
		if s.DisableDirectorySearch {
			go s.searchDirectoryFlat(&wg, path, channel)
		} else {
			go s.searchDirectory(&wg, path, channel)
		}
	}
	wg.Wait()
}

func (s *FilesystemStore) searchDirectory(
	wg *sync.WaitGroup,
	searchPath string,
	channel chan storetypes.SearchResult,
) {
	defer wg.Done()

	if err := godirwalk.Walk(searchPath, &godirwalk.Options{
		Callback: func(osPathname string, _ *godirwalk.Dirent) error {
			fileName := filepath.Base(osPathname)
			matched, err := filepath.Match(s.KubeconfigName, fileName)
			if err != nil {
				return err
			}
			if matched {
				channel <- storetypes.SearchResult{
					KubeconfigPath: osPathname,
					Error:          nil,
				}
			}
			return nil
		},
		Unsorted:            false, // (optional) set true for faster yet non-deterministic enumeration
		FollowSymbolicLinks: true,
	}); err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("failed to find kubeconfig files in directory: %v", err),
		}
	}
}

func (s *FilesystemStore) searchDirectoryFlat(
	wg *sync.WaitGroup,
	searchPath string,
	channel chan storetypes.SearchResult,
) {
	defer wg.Done()

	entries, err := os.ReadDir(searchPath)
	if err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("failed to read directory %q: %v", searchPath, err),
		}
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matched, err := filepath.Match(s.KubeconfigName, entry.Name())
		if err != nil {
			channel <- storetypes.SearchResult{
				KubeconfigPath: "",
				Error:          err,
			}
			return
		}
		if matched {
			channel <- storetypes.SearchResult{
				KubeconfigPath: filepath.Join(searchPath, entry.Name()),
				Error:          nil,
			}
		}
	}
}

func (s *FilesystemStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	return os.ReadFile(path)
}

func (s *FilesystemStore) VerifyKubeconfigPaths() error {
	var (
		duplicatePath              = make(map[string]*struct{})
		validKubeconfigFilepaths   []string
		validKubeconfigDirectories []string
		usr, _                     = user.Current()
		homeDir                    = usr.HomeDir
	)

	for _, path := range s.KubeconfigStore.Paths {
		// do not add duplicate paths
		if duplicatePath[path] != nil {
			continue
		}
		duplicatePath[path] = &struct{}{}

		kubeconfigPath := path
		if kubeconfigPath == "~" {
			kubeconfigPath = homeDir
		} else if strings.HasPrefix(kubeconfigPath, "~/") {
			// Use strings.HasPrefix so we don't match paths like
			// "/something/~/something/"
			kubeconfigPath = filepath.Join(homeDir, kubeconfigPath[2:])
		}

		info, err := os.Stat(kubeconfigPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to read from the configured kubeconfig directory %q: %v", path, err)
		}

		if info.IsDir() {
			validKubeconfigDirectories = append(validKubeconfigDirectories, kubeconfigPath)
			continue
		}
		validKubeconfigFilepaths = append(validKubeconfigFilepaths, kubeconfigPath)
	}

	if len(validKubeconfigDirectories) == 0 && len(validKubeconfigFilepaths) == 0 {
		return fmt.Errorf(
			"none of the %d specified kubeconfig path(s) exist. Either specifiy an existing path via flag '--kubeconfig-path' or in the switch config file",
			len(s.KubeconfigStore.Paths),
		)
	}
	s.kubeconfigDirectories = validKubeconfigDirectories
	s.kubeconfigFilepaths = validKubeconfigFilepaths
	return nil
}

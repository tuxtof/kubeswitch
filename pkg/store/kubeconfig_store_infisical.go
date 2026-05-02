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
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	infisical "github.com/infisical/go-sdk"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	storetypes "github.com/danielfoehrkn/kubeswitch/pkg/store/types"
	"github.com/danielfoehrkn/kubeswitch/types"
)

func NewInfisicalStore(kubeconfigStore types.KubeconfigStore) (*InfisicalStore, error) {
	infisicalConfig := &types.StoreConfigInfisical{}
	if kubeconfigStore.Config != nil {
		buf, err := yaml.Marshal(kubeconfigStore.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal infisical config: %w", err)
		}

		err = yaml.Unmarshal(buf, infisicalConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal infisical config: %w", err)
		}
	}

	// Allow environment variables to override config
	if envSiteURL := os.Getenv("INFISICAL_SITE_URL"); envSiteURL != "" {
		infisicalConfig.SiteURL = envSiteURL
	}
	if envClientID := os.Getenv("INFISICAL_CLIENT_ID"); envClientID != "" {
		infisicalConfig.ClientID = envClientID
	}
	if envClientSecret := os.Getenv("INFISICAL_CLIENT_SECRET"); envClientSecret != "" {
		infisicalConfig.ClientSecret = envClientSecret
	}
	if envProjectID := os.Getenv("INFISICAL_PROJECT_ID"); envProjectID != "" {
		infisicalConfig.ProjectID = envProjectID
	}
	if envEnvironment := os.Getenv("INFISICAL_ENVIRONMENT"); envEnvironment != "" {
		infisicalConfig.Environment = envEnvironment
	}

	if infisicalConfig.ClientID == "" || infisicalConfig.ClientSecret == "" {
		return nil, fmt.Errorf("when using the infisical kubeconfig store, both clientID and clientSecret must be provided via configuration or environment variables INFISICAL_CLIENT_ID and INFISICAL_CLIENT_SECRET")
	}

	if infisicalConfig.ProjectID == "" {
		return nil, fmt.Errorf("when using the infisical kubeconfig store, projectID must be provided via configuration or environment variable INFISICAL_PROJECT_ID")
	}

	if infisicalConfig.Environment == "" {
		return nil, fmt.Errorf("when using the infisical kubeconfig store, environment must be provided via configuration or environment variable INFISICAL_ENVIRONMENT")
	}

	// Default values
	siteURL := infisicalConfig.SiteURL
	if siteURL == "" {
		siteURL = "https://app.infisical.com"
	}

	if infisicalConfig.SecretPath == "" {
		infisicalConfig.SecretPath = "/"
	}

	client := infisical.NewInfisicalClient(context.Background(), infisical.Config{
		SiteUrl: siteURL,
	})

	// Authenticate using Universal Auth
	_, err := client.Auth().UniversalAuthLogin(infisicalConfig.ClientID, infisicalConfig.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with Infisical: %w", err)
	}

	return &InfisicalStore{
		Logger:          logrus.New().WithField("store", types.StoreKindInfisical),
		KubeconfigStore: kubeconfigStore,
		Config:          infisicalConfig,
		Client:          client,
	}, nil
}

func (s *InfisicalStore) GetID() string {
	id := "default"
	if s.KubeconfigStore.ID != nil {
		id = *s.KubeconfigStore.ID
	}
	return fmt.Sprintf("%s.%s", types.StoreKindInfisical, id)
}

func (s *InfisicalStore) GetContextPrefix(path string) string {
	if s.GetStoreConfig().ShowPrefix != nil && !*s.GetStoreConfig().ShowPrefix {
		return ""
	}
	return filepath.Base(path)
}

func (s *InfisicalStore) GetKind() types.StoreKind {
	return types.StoreKindInfisical
}

func (s *InfisicalStore) GetStoreConfig() types.KubeconfigStore {
	return s.KubeconfigStore
}

func (s *InfisicalStore) GetLogger() *logrus.Entry {
	return s.Logger
}

func (s *InfisicalStore) StartSearch(channel chan storetypes.SearchResult) {
	secretPath := s.Config.SecretPath

	// If a specific secret key is configured, only return that one path
	if s.Config.SecretKey != "" {
		kubeconfigPath := fmt.Sprintf("%s/%s", secretPath, s.Config.SecretKey)
		channel <- storetypes.SearchResult{
			KubeconfigPath: kubeconfigPath,
			Error:          nil,
		}
		s.Logger.Debugf("Found %s", kubeconfigPath)
		return
	}

	// List all secrets in the path
	result, err := s.Client.Secrets().ListSecrets(infisical.ListSecretsOptions{
		ProjectID:   s.Config.ProjectID,
		Environment: s.Config.Environment,
		SecretPath:  secretPath,
		Recursive:   true,
	})
	if err != nil {
		channel <- storetypes.SearchResult{
			KubeconfigPath: "",
			Error:          fmt.Errorf("failed to list secrets from Infisical: %w", err),
		}
		return
	}

	for _, secret := range result.Secrets {
		kubeconfigPath := fmt.Sprintf("%s/%s", secret.SecretPath, secret.SecretKey)
		channel <- storetypes.SearchResult{
			KubeconfigPath: kubeconfigPath,
			Error:          nil,
		}
		s.Logger.Debugf("Found %s", kubeconfigPath)
	}
}

func (s *InfisicalStore) GetKubeconfigForPath(path string, _ map[string]string) ([]byte, error) {
	// Extract the secret key from the path (last segment)
	secretKey := filepath.Base(path)
	// Extract the secret path (directory part)
	secretPath := filepath.Dir(path)

	s.Logger.Debugf("infisical: getting secret key %q from path %q", secretKey, secretPath)

	secret, err := s.Client.Secrets().Retrieve(infisical.RetrieveSecretOptions{
		SecretKey:   secretKey,
		ProjectID:   s.Config.ProjectID,
		Environment: s.Config.Environment,
		SecretPath:  secretPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret %q from Infisical: %w", path, err)
	}

	if secret.SecretValue == "" {
		return nil, fmt.Errorf("secret %q is empty in Infisical", path)
	}

	data := []byte(secret.SecretValue)

	// Check if value is base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(secret.SecretValue)
	if err == nil {
		data = decoded
	}

	return data, nil
}

func (s *InfisicalStore) VerifyKubeconfigPaths() error {
	// Verify we can access the project by listing secrets
	_, err := s.Client.Secrets().ListSecrets(infisical.ListSecretsOptions{
		ProjectID:   s.Config.ProjectID,
		Environment: s.Config.Environment,
		SecretPath:  s.Config.SecretPath,
	})
	if err != nil {
		return fmt.Errorf("failed to verify Infisical access for project %q, environment %q, path %q: %w",
			s.Config.ProjectID, s.Config.Environment, s.Config.SecretPath, err)
	}
	return nil
}

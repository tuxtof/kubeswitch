# Use the Infisical store

[Infisical](https://infisical.com) is an open-source secret management platform.
Kubeswitch supports using Infisical as a backing store for kubeconfig files,
allowing you to store and retrieve kubeconfigs as secrets in Infisical.

## Prerequisites

- An Infisical account and project
- A Machine Identity with Universal Auth configured (see [Infisical docs](https://infisical.com/docs/documentation/platform/identities/universal-auth))
- The Machine Identity must have read access to the secrets containing kubeconfigs

## Authentication

Kubeswitch authenticates to Infisical using **Universal Auth** (Client ID + Client Secret).

Credentials can be provided via:
1. The `SwitchConfig` file (fields `clientID` and `clientSecret`)
2. Environment variables `INFISICAL_CLIENT_ID` and `INFISICAL_CLIENT_SECRET`

Environment variables take precedence over config file values.

## Storing Kubeconfigs in Infisical

Store each kubeconfig as a secret in your Infisical project. The secret value should be
the raw kubeconfig YAML content (or base64-encoded kubeconfig).

For example, you might create secrets like:
- Secret key: `dev-cluster` with value being the kubeconfig content
- Secret key: `prod-cluster` with value being the kubeconfig content

These can be organized in folders/paths within your Infisical project.

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `INFISICAL_SITE_URL` | URL of the Infisical instance (defaults to `https://app.infisical.com`) |
| `INFISICAL_CLIENT_ID` | Client ID for Universal Auth |
| `INFISICAL_CLIENT_SECRET` | Client Secret for Universal Auth |
| `INFISICAL_PROJECT_ID` | Infisical project ID |
| `INFISICAL_ENVIRONMENT` | Environment slug (e.g., `dev`, `staging`, `prod`) |

### SwitchConfig file

Basic configuration:

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: infisical
  config:
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<your-project-id>"
    environment: "prod"
    secretPath: "/"
```

### Configuration Options

| Field | Description | Default |
|-------|-------------|---------|
| `siteURL` | URL of the Infisical instance | `https://app.infisical.com` |
| `clientID` | Client ID for Universal Auth (required) | - |
| `clientSecret` | Client Secret for Universal Auth (required) | - |
| `projectID` | Infisical project ID (required) | - |
| `environment` | Environment slug (required) | - |
| `secretPath` | Path within the project to search for secrets | `/` |
| `secretKey` | Specific secret key to retrieve (if only one kubeconfig) | - |

### Retrieve a single kubeconfig

If you only have one kubeconfig stored in Infisical, you can specify the exact secret key:

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: infisical
  config:
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<your-project-id>"
    environment: "prod"
    secretPath: "/kubernetes"
    secretKey: "my-cluster-kubeconfig"
```

### Retrieve all kubeconfigs from a path

To discover all secrets in a given path (treating each as a kubeconfig):

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: infisical
  config:
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<your-project-id>"
    environment: "prod"
    secretPath: "/kubernetes/kubeconfigs"
```

### Self-hosted Infisical

For self-hosted Infisical instances, set the `siteURL`:

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: infisical
  config:
    siteURL: "https://infisical.your-company.com"
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<your-project-id>"
    environment: "prod"
    secretPath: "/"
```

### Combined with cache

You can combine the Infisical store with caching to reduce API calls:

```yaml
kind: SwitchConfig
version: v1alpha1
refreshIndexAfter: 12h
kubeconfigStores:
- kind: infisical
  config:
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<your-project-id>"
    environment: "prod"
    secretPath: "/"
  cache:
    kind: filesystem
    config:
      path: ~/.kube/cache/switch
```

### Multiple Infisical stores

You can configure multiple Infisical stores (e.g., for different projects or environments):

```yaml
kind: SwitchConfig
version: v1alpha1
kubeconfigStores:
- kind: infisical
  id: dev-clusters
  config:
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<dev-project-id>"
    environment: "dev"
    secretPath: "/kubernetes"
- kind: infisical
  id: prod-clusters
  config:
    clientID: "<your-client-id>"
    clientSecret: "<your-client-secret>"
    projectID: "<prod-project-id>"
    environment: "prod"
    secretPath: "/kubernetes"
```

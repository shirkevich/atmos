package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	log "github.com/charmbracelet/log"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
)

const (
	gsmOperationTimeout = 30 * time.Second
	gsmKeySeparator    = "_"
)

// GSMClient is the interface that wraps the Google Secret Manager client methods we use
type GSMClient interface {
	CreateSecret(ctx context.Context, req *secretmanagerpb.CreateSecretRequest, opts ...gax.CallOption) (*secretmanagerpb.Secret, error)
	AddSecretVersion(ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.SecretVersion, error)
	AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)
	Close() error
}

// GSMStore is an implementation of the Store interface for Google Secret Manager.
type GSMStore struct {
	client         GSMClient
	projectID      string
	prefix         string
	stackDelimiter *string
}

type GSMStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	ProjectID      string  `mapstructure:"project_id"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	Credentials    *string `mapstructure:"credentials"` // Optional JSON credentials
}

// Ensure GSMStore implements the store.Store interface.
var _ Store = (*GSMStore)(nil)

// NewGSMStore initializes a new Google Secret Manager Store.
func NewGSMStore(options GSMStoreOptions) (Store, error) {
	ctx := context.Background()

	if options.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required in Google Secret Manager store configuration")
	}

	var clientOpts []option.ClientOption
	if options.Credentials != nil {
		clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(*options.Credentials)))
	}

	client, err := secretmanager.NewClient(ctx, clientOpts...)
	if err != nil {
		// Close the client to prevent resource leaks
		if client != nil {
			client.Close()
		}
		return nil, fmt.Errorf("failed to create Secret Manager client: %w", err)
	}

	store := &GSMStore{
		client:    client,
		projectID: options.ProjectID,
	}

	if options.Prefix != nil {
		store.prefix = *options.Prefix
	}

	if options.StackDelimiter != nil {
		store.stackDelimiter = options.StackDelimiter
	} else {
		defaultDelimiter := "-"
		store.stackDelimiter = &defaultDelimiter
	}

	return store, nil
}

func (s *GSMStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", fmt.Errorf("stack delimiter is not set")
	}

	// Get the base key using the common getKey function
	baseKey, err := getKey(s.prefix, *s.stackDelimiter, stack, component, key, gsmKeySeparator)
	if err != nil {
		return "", err
	}

	// Replace any remaining slashes with underscores as Secret Manager doesn't allow slashes
	baseKey = strings.ReplaceAll(baseKey, "/", gsmKeySeparator)
	// Remove any double underscores that might have been created
	baseKey = strings.ReplaceAll(baseKey, gsmKeySeparator+gsmKeySeparator, gsmKeySeparator)
	// Trim any leading or trailing underscores
	baseKey = strings.Trim(baseKey, gsmKeySeparator)

	return baseKey, nil
}

func (s *GSMStore) createSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error) {
	parent := fmt.Sprintf("projects/%s", s.projectID)
	createSecretReq := &secretmanagerpb.CreateSecretRequest{
		Parent:   parent,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}

	secret, err := s.client.CreateSecret(ctx, createSecretReq)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			log.Debug("secret already exists",
				"project", s.projectID,
				"secret_id", secretID)
			return &secretmanagerpb.Secret{
				Name: fmt.Sprintf("projects/%s/secrets/%s", s.projectID, secretID),
			}, nil
		}
		log.Debug("failed to create secret",
			"project", s.projectID,
			"secret_id", secretID,
			"error", err)
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}
	log.Debug("successfully created secret",
		"name", secret.GetName())
	return secret, nil
}

func (s *GSMStore) addSecretVersion(ctx context.Context, secret *secretmanagerpb.Secret, value string) error {
	addVersionReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: secret.GetName(),
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	}

	log.Debug("adding new version to secret",
		"name", secret.GetName())

	_, err := s.client.AddSecretVersion(ctx, addVersionReq)
	if err != nil {
		log.Debug("failed to add version to secret",
			"name", secret.GetName(),
			"error", err)
		return fmt.Errorf("failed to add secret version: %w", err)
	}
	return nil
}

// Set stores a key-value pair in Google Secret Manager.
func (s *GSMStore) Set(stack string, component string, key string, value interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	if stack == "" {
		return fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	log.Debug("creating/updating Google Secret Manager secret",
		"project", s.projectID,
		"secret_id", secretID,
		"stack", stack,
		"component", component,
		"key", key)

	secret, err := s.createSecret(ctx, secretID)
	if err != nil {
		return err
	}

	if err := s.addSecretVersion(ctx, secret, strValue); err != nil {
		return err
	}

	log.Debug("successfully added new version to secret",
		"name", secret.GetName())
	return nil
}

// Get retrieves a value by key from Google Secret Manager.
func (s *GSMStore) Get(stack string, component string, key string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	if stack == "" {
		return nil, fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return nil, fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	// Get the secret ID using getKey
	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	// Build the resource name for the latest version
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", s.projectID, secretID)

	log.Debug("retrieving Google Secret Manager secret",
		"name", name,
		"project", s.projectID,
		"secret_id", secretID,
		"stack", stack,
		"component", component,
		"key", key)

	// Access the secret version
	result, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		log.Debug("failed to retrieve secret",
			"name", name,
			"error", err)
		return nil, fmt.Errorf("failed to access secret version: %w", err)
	}

	log.Debug("successfully retrieved secret",
		"name", name)
	return string(result.Payload.Data), nil
}

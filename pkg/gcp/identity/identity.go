package identity

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	oauth2v2 "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultADCScope is the default OAuth scope used when calling ADC.
const defaultADCScope = "https://www.googleapis.com/auth/cloud-platform"

// CallerIdentity holds the GCP caller identity information.
type CallerIdentity struct {
	// ProjectID is the GCP project ID.
	ProjectID string
	// Region is the GCP region.
	Region string
	// ServiceAccountEmail is the service account email (if available).
	ServiceAccountEmail string
}

// Getter provides an interface for retrieving GCP caller identity information.
// This interface enables dependency injection and testability.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_identity.go -package=identity
type Getter interface {
	// GetCallerIdentity retrieves the GCP caller identity for the current credentials.
	// Returns the project ID, region, and service account email.
	GetCallerIdentity(
		ctx context.Context,
		atmosConfig *schema.AtmosConfiguration,
		gcpAuth *schema.GCPAuthContext,
	) (*CallerIdentity, error)
}

// defaultGetter is the production implementation.
type defaultGetter struct{}

// GetCallerIdentity retrieves GCP caller identity using the following priority:
//  1. Atmos GCPAuthContext (populated by atmos auth login).
//  2. Environment variables (GOOGLE_CLOUD_PROJECT, GOOGLE_CLOUD_REGION, etc.).
//  3. Active gcloud SDK configuration file (~/.config/gcloud/configurations/config_<name>).
//  4. Application Default Credentials (project ID and service account email).
func (d *defaultGetter) GetCallerIdentity(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	gcpAuth *schema.GCPAuthContext,
) (*CallerIdentity, error) {
	defer perf.Track(atmosConfig, "identity.Getter.GetCallerIdentity")()

	log.Debug("Getting GCP caller identity")

	identity := &CallerIdentity{}

	// Priority 1: Use Atmos GCPAuthContext when available.
	if gcpAuth != nil {
		identity.ProjectID = gcpAuth.ProjectID
		identity.Region = gcpAuth.Region
		identity.ServiceAccountEmail = gcpAuth.ServiceAccountEmail
		log.Debug("Using GCP auth context for identity",
			"project_id", identity.ProjectID,
			"region", identity.Region,
			"service_account_email", identity.ServiceAccountEmail,
		)
	}

	// Priority 2: Fall back to environment variables.
	if identity.ProjectID == "" {
		identity.ProjectID = getProjectFromEnv()
	}
	if identity.Region == "" {
		identity.Region = getRegionFromEnv()
	}

	// Priority 3: Fall back to active gcloud SDK configuration file.
	if identity.ProjectID == "" {
		identity.ProjectID = readGcloudConfig("core", "project")
	}
	if identity.Region == "" {
		identity.Region = readGcloudConfig("compute", "region")
	}

	// Priority 4: Fall back to ADC for project ID and service account email.
	if identity.ProjectID == "" || identity.ServiceAccountEmail == "" {
		if err := enrichFromADC(ctx, identity); err != nil {
			// ADC is best-effort; log but do not fail.
			log.Debug("ADC fallback failed (non-fatal)", "error", err)
		}
	}

	if identity.ProjectID == "" {
		return nil, fmt.Errorf("%w: no GCP project ID found in auth context, environment variables, or ADC", errUtils.ErrGCPGetCallerIdentity)
	}

	log.Debug("Resolved GCP caller identity",
		"project_id", identity.ProjectID,
		"region", identity.Region,
		"service_account_email", identity.ServiceAccountEmail,
	)

	return identity, nil
}

// getProjectFromEnv returns the GCP project ID from well-known environment variables.
func getProjectFromEnv() string {
	for _, key := range []string{"GOOGLE_CLOUD_PROJECT", "GCLOUD_PROJECT", "CLOUDSDK_CORE_PROJECT"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// getRegionFromEnv returns the GCP region from well-known environment variables.
func getRegionFromEnv() string {
	for _, key := range []string{"GOOGLE_CLOUD_REGION", "CLOUDSDK_COMPUTE_REGION"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// gcloudConfigDir returns the path to the gcloud configuration directory.
// Respects CLOUDSDK_CONFIG env var override used by the gcloud SDK.
func gcloudConfigDir() string {
	if dir := os.Getenv("CLOUDSDK_CONFIG"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gcloud")
}

// readGcloudConfig reads a key from a section in the active gcloud configuration file.
// The gcloud config file is INI-formatted. Section headers are [section], keys are "key = value".
func readGcloudConfig(section, key string) string {
	configDir := gcloudConfigDir()
	if configDir == "" {
		return ""
	}

	// Determine active configuration name (defaults to "default").
	configName := "default"
	activeFile := filepath.Join(configDir, "active_config")
	if data, err := os.ReadFile(activeFile); err == nil {
		if name := strings.TrimSpace(string(data)); name != "" {
			configName = name
		}
	}
	if v := os.Getenv("CLOUDSDK_ACTIVE_CONFIG"); v != "" {
		configName = v
	}

	configFile := filepath.Join(configDir, "configurations", "config_"+configName)
	f, err := os.Open(configFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Parse the INI-style config file.
	inSection := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inSection = strings.EqualFold(line, "["+section+"]")
			continue
		}
		if inSection {
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// enrichFromADC attempts to populate missing identity fields from Application Default Credentials.
// Errors are non-fatal since ADC may not be configured.
func enrichFromADC(ctx context.Context, identity *CallerIdentity) error {
	defer perf.Track(nil, "identity.enrichFromADC")()

	creds, err := google.FindDefaultCredentials(ctx, defaultADCScope)
	if err != nil {
		return err
	}

	if identity.ProjectID == "" && creds.ProjectID != "" {
		identity.ProjectID = creds.ProjectID
	}

	// Try to get service account email from tokeninfo when an access token is available.
	if identity.ServiceAccountEmail == "" {
		token, err := creds.TokenSource.Token()
		if err == nil && token.AccessToken != "" {
			email, err := getTokenEmail(ctx, token.AccessToken)
			if err == nil {
				identity.ServiceAccountEmail = email
			}
		}
	}

	return nil
}

// getTokenEmail retrieves the email associated with an access token via the OAuth2 tokeninfo API.
func getTokenEmail(ctx context.Context, accessToken string) (string, error) {
	defer perf.Track(nil, "identity.getTokenEmail")()

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	svc, err := oauth2v2.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", err
	}

	tokenInfo, err := svc.Tokeninfo().AccessToken(accessToken).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return tokenInfo.Email, nil
}

// getter is the global instance. Tests can replace it via SetGetter.
var getter Getter = &defaultGetter{}

// SetGetter allows tests to inject a mock Getter.
// Returns a function that restores the original getter.
func SetGetter(g Getter) func() {
	defer perf.Track(nil, "identity.SetGetter")()

	original := getter
	getter = g
	return func() {
		getter = original
	}
}

// cachedIdentity holds a cached GCP caller identity result.
type cachedIdentity struct {
	identity *CallerIdentity
	err      error
}

var (
	identityCache   map[string]*cachedIdentity
	identityCacheMu sync.RWMutex
)

func init() {
	identityCache = make(map[string]*cachedIdentity)
}

// getCacheKey generates a cache key from the GCP auth context.
// Different auth contexts produce different cache entries.
func getCacheKey(gcpAuth *schema.GCPAuthContext) string {
	if gcpAuth == nil {
		return "default"
	}
	return fmt.Sprintf("%s|%s|%s", gcpAuth.ProjectID, gcpAuth.Region, gcpAuth.ServiceAccountEmail)
}

// GetCallerIdentityCached retrieves the GCP caller identity with in-process caching.
// Results are cached per auth context to avoid repeated calls within the same CLI invocation.
func GetCallerIdentityCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	gcpAuth *schema.GCPAuthContext,
) (*CallerIdentity, error) {
	defer perf.Track(atmosConfig, "identity.GetCallerIdentityCached")()

	cacheKey := getCacheKey(gcpAuth)

	// Check cache (read lock).
	identityCacheMu.RLock()
	if cached, ok := identityCache[cacheKey]; ok {
		identityCacheMu.RUnlock()
		log.Debug("Using cached GCP caller identity", "cache_key", cacheKey)
		return cached.identity, cached.err
	}
	identityCacheMu.RUnlock()

	// Cache miss — acquire write lock.
	identityCacheMu.Lock()
	defer identityCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := identityCache[cacheKey]; ok {
		log.Debug("Using cached GCP caller identity (double-check)", "cache_key", cacheKey)
		return cached.identity, cached.err
	}

	// Fetch identity.
	identity, err := getter.GetCallerIdentity(ctx, atmosConfig, gcpAuth)

	// Cache result (including errors to avoid repeated failed calls).
	identityCache[cacheKey] = &cachedIdentity{
		identity: identity,
		err:      err,
	}

	return identity, err
}

// ClearIdentityCache clears the in-process GCP identity cache.
// Useful in tests or when credentials change during execution.
func ClearIdentityCache() {
	defer perf.Track(nil, "identity.ClearIdentityCache")()

	identityCacheMu.Lock()
	defer identityCacheMu.Unlock()
	identityCache = make(map[string]*cachedIdentity)
}

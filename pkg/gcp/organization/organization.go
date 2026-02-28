package organization

import (
	"context"
	"fmt"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"

	errUtils "github.com/cloudposse/atmos/errors"
	internalGCP "github.com/cloudposse/atmos/internal/gcp"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// crmProjectsService defines the subset of the Cloud Resource Manager ProjectsService methods
// used by this package. This interface enables unit testing without real GCP credentials.
type crmProjectsService interface {
	GetAncestry(projectID string, req *cloudresourcemanager.GetAncestryRequest) (*cloudresourcemanager.GetAncestryResponse, error)
}

// crmProjectsServiceAdapter wraps the real CloudResourceManager ProjectsService to implement crmProjectsService.
type crmProjectsServiceAdapter struct {
	svc *cloudresourcemanager.ProjectsService
	ctx context.Context
}

// GetAncestry calls the real GetAncestry API with the context from the adapter.
func (a *crmProjectsServiceAdapter) GetAncestry(projectID string, req *cloudresourcemanager.GetAncestryRequest) (*cloudresourcemanager.GetAncestryResponse, error) {
	return a.svc.GetAncestry(projectID, req).Context(a.ctx).Do()
}

// crmClientFactory is the type for a factory function that creates a crmProjectsService.
// This is a package-level variable to enable testing without real GCP credentials.
type crmClientFactoryFn func(ctx context.Context, gcpAuth *schema.GCPAuthContext) (crmProjectsService, error)

// newCRMClient is the default factory for creating Cloud Resource Manager project service clients.
// This is a package-level variable to enable testing without real GCP credentials.
var newCRMClient crmClientFactoryFn = func(ctx context.Context, gcpAuth *schema.GCPAuthContext) (crmProjectsService, error) {
	clientOpts := buildClientOptions(gcpAuth)
	svc, err := cloudresourcemanager.NewService(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Resource Manager client: %w", err)
	}
	return &crmProjectsServiceAdapter{svc: svc.Projects, ctx: ctx}, nil
}

// buildClientOptions constructs Google Cloud client options from the GCPAuthContext.
// If an explicit access token is provided in gcpAuth, it takes precedence over all other auth methods.
// Otherwise, it falls back to the standard GetClientOptions logic (credentials file, env vars, ADC).
func buildClientOptions(gcpAuth *schema.GCPAuthContext) []option.ClientOption {
	if gcpAuth != nil && gcpAuth.AccessToken != "" {
		token := &oauth2.Token{AccessToken: gcpAuth.AccessToken}
		return []option.ClientOption{option.WithTokenSource(oauth2.StaticTokenSource(token))}
	}

	authOpts := internalGCP.AuthOptions{}
	if gcpAuth != nil && gcpAuth.CredentialsFile != "" {
		authOpts.Credentials = gcpAuth.CredentialsFile
	}
	return internalGCP.GetClientOptions(authOpts)
}

// getProjectID extracts the GCP project ID from gcpAuth or falls back to the GOOGLE_CLOUD_PROJECT env var.
func getProjectID(gcpAuth *schema.GCPAuthContext) string {
	if gcpAuth != nil && gcpAuth.ProjectID != "" {
		return gcpAuth.ProjectID
	}
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

// OrganizationInfo holds the information returned by the GCP organization lookup.
type OrganizationInfo struct {
	// ID is the numeric organization ID (e.g. "123456789"), without the "organizations/" prefix.
	ID string
	// DisplayName is the human-readable display name of the organization (may be empty).
	DisplayName string
	// Name is the full resource name of the organization (e.g. "organizations/123456789").
	Name string
}

// Getter provides an interface for retrieving GCP organization information.
// This interface enables dependency injection and testability.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_organization.go -package=organization
type Getter interface {
	// GetOrganization retrieves the GCP organization info for the given project.
	// It walks the project's resource ancestry to find the organization ancestor.
	GetOrganization(
		ctx context.Context,
		atmosConfig *schema.AtmosConfiguration,
		gcpAuth *schema.GCPAuthContext,
	) (*OrganizationInfo, error)
}

// defaultGetter is the production implementation that uses real GCP SDK calls.
type defaultGetter struct{}

// GetOrganization retrieves the GCP organization info by walking the project's resource ancestry.
func (d *defaultGetter) GetOrganization(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	gcpAuth *schema.GCPAuthContext,
) (*OrganizationInfo, error) {
	defer perf.Track(atmosConfig, "organization.Getter.GetOrganization")()

	projectID := getProjectID(gcpAuth)
	if projectID == "" {
		return nil, fmt.Errorf("%w: set project_id in GCP auth context or GOOGLE_CLOUD_PROJECT env var", errUtils.ErrGCPProjectIDRequired)
	}

	log.Debug("Getting GCP organization info", "project_id", projectID)

	svc, err := newCRMClient(ctx, gcpAuth)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGCPDescribeOrganization, err)
	}

	resp, err := svc.GetAncestry(projectID, &cloudresourcemanager.GetAncestryRequest{})
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGCPDescribeOrganization, err)
	}

	// Walk the ancestry list to find the organization ancestor.
	for _, ancestor := range resp.Ancestor {
		if ancestor.ResourceId == nil {
			continue
		}
		if ancestor.ResourceId.Type == "organization" {
			orgID := ancestor.ResourceId.Id
			info := &OrganizationInfo{
				ID:   orgID,
				Name: "organizations/" + orgID,
			}
			log.Debug("Retrieved GCP organization info", "org_id", orgID, "project_id", projectID)
			return info, nil
		}
	}

	return nil, fmt.Errorf("%w: project %q has no organization ancestor (personal or standalone project)", errUtils.ErrGCPDescribeOrganization, projectID)
}

// getter is the global instance used by package-level functions.
// This allows test code to replace it with a mock.
var getter Getter = &defaultGetter{}

// SetGetter allows tests to inject a mock Getter.
// Returns a function to restore the original getter.
func SetGetter(g Getter) func() {
	defer perf.Track(nil, "organization.SetGetter")()

	original := getter
	if g == nil {
		getter = &defaultGetter{}
	} else {
		getter = g
	}
	return func() {
		getter = original
	}
}

// cachedOrganization holds the cached GCP organization info.
// The cache is per-CLI-invocation (stored in memory) to avoid repeated API calls.
type cachedOrganization struct {
	info *OrganizationInfo
	err  error
}

var (
	organizationCache   map[string]*cachedOrganization
	organizationCacheMu sync.RWMutex
)

func init() {
	organizationCache = make(map[string]*cachedOrganization)
}

// getCacheKey generates a cache key based on the project ID.
// Different project IDs get different cache entries.
func getCacheKey(gcpAuth *schema.GCPAuthContext) string {
	defer perf.Track(nil, "organization.getCacheKey")()

	projectID := getProjectID(gcpAuth)
	if projectID == "" {
		return "default"
	}
	return projectID
}

// GetOrganizationCached retrieves the GCP organization info with caching.
// Results are cached per project ID to avoid repeated API calls within the same CLI invocation.
func GetOrganizationCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	gcpAuth *schema.GCPAuthContext,
) (*OrganizationInfo, error) {
	defer perf.Track(atmosConfig, "organization.GetOrganizationCached")()

	cacheKey := getCacheKey(gcpAuth)

	// Check cache first (read lock).
	organizationCacheMu.RLock()
	if cached, ok := organizationCache[cacheKey]; ok {
		organizationCacheMu.RUnlock()
		log.Debug("Using cached GCP organization info", "cache_key", cacheKey)
		return cached.info, cached.err
	}
	organizationCacheMu.RUnlock()

	// Cache miss - acquire write lock and fetch.
	organizationCacheMu.Lock()
	defer organizationCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := organizationCache[cacheKey]; ok {
		log.Debug("Using cached GCP organization info (double-check)", "cache_key", cacheKey)
		return cached.info, cached.err
	}

	// Fetch from GCP.
	info, err := getter.GetOrganization(ctx, atmosConfig, gcpAuth)

	// Cache the result (including errors to avoid repeated failed calls).
	organizationCache[cacheKey] = &cachedOrganization{
		info: info,
		err:  err,
	}

	return info, err
}

// ClearOrganizationCache clears the GCP organization cache.
// This is useful in tests or when credentials change during execution.
func ClearOrganizationCache() {
	defer perf.Track(nil, "organization.ClearOrganizationCache")()

	organizationCacheMu.Lock()
	defer organizationCacheMu.Unlock()
	organizationCache = make(map[string]*cachedOrganization)
}

package exec

import (
	"context"

	gcpIdentity "github.com/cloudposse/atmos/pkg/gcp/identity"
	gcpOrg "github.com/cloudposse/atmos/pkg/gcp/organization"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GCPCallerIdentity holds the information returned by the GCP identity lookup.
// This is a type alias that delegates to pkg/gcp/identity.CallerIdentity.
type GCPCallerIdentity = gcpIdentity.CallerIdentity

// GCPGetter provides an interface for retrieving GCP caller identity information.
// This interface enables dependency injection and testability.
// This is a type alias that delegates to pkg/gcp/identity.Getter.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_gcp_getter_test.go -package=exec
type GCPGetter = gcpIdentity.Getter

// SetGCPGetter allows tests to inject a mock GCPGetter.
// Returns a function to restore the original getter.
func SetGCPGetter(getter GCPGetter) func() {
	defer perf.Track(nil, "exec.SetGCPGetter")()

	return gcpIdentity.SetGetter(getter)
}

// getGCPCallerIdentityCached retrieves the GCP caller identity with caching.
// Results are cached per auth context to avoid repeated API calls within the same CLI invocation.
func getGCPCallerIdentityCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.GCPAuthContext,
) (*GCPCallerIdentity, error) {
	defer perf.Track(atmosConfig, "exec.getGCPCallerIdentityCached")()

	return gcpIdentity.GetCallerIdentityCached(ctx, atmosConfig, authContext)
}

// ClearGCPIdentityCache clears the GCP identity cache.
// This is useful in tests or when credentials change during execution.
func ClearGCPIdentityCache() {
	defer perf.Track(nil, "exec.ClearGCPIdentityCache")()

	gcpIdentity.ClearIdentityCache()
}

// GCPOrganizationInfo holds the information returned by the GCP organization lookup.
// This is a type alias that delegates to pkg/gcp/organization.OrganizationInfo.
type GCPOrganizationInfo = gcpOrg.OrganizationInfo

// GCPOrganizationGetter provides an interface for retrieving GCP organization information.
// This is a type alias that delegates to pkg/gcp/organization.Getter.
type GCPOrganizationGetter = gcpOrg.Getter

// SetGCPOrganizationGetter allows tests to inject a mock GCPOrganizationGetter.
// Returns a function to restore the original getter.
func SetGCPOrganizationGetter(getter GCPOrganizationGetter) func() {
	defer perf.Track(nil, "exec.SetGCPOrganizationGetter")()

	return gcpOrg.SetGetter(getter)
}

// getGCPOrganizationCached retrieves the GCP organization info with caching.
// Results are cached per auth context to avoid repeated API calls within the same CLI invocation.
func getGCPOrganizationCached(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	authContext *schema.GCPAuthContext,
) (*GCPOrganizationInfo, error) {
	defer perf.Track(atmosConfig, "exec.getGCPOrganizationCached")()

	return gcpOrg.GetOrganizationCached(ctx, atmosConfig, authContext)
}

// ClearGCPOrganizationCache clears the GCP organization cache.
// This is useful in tests or when credentials change during execution.
func ClearGCPOrganizationCache() {
	defer perf.Track(nil, "exec.ClearGCPOrganizationCache")()

	gcpOrg.ClearOrganizationCache()
}

package function

import (
	"context"

	gcpIdentity "github.com/cloudposse/atmos/pkg/gcp/identity"
	gcpOrg "github.com/cloudposse/atmos/pkg/gcp/organization"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errMsgGCPIdentityFailed is a constant for the GCP identity error message.
const errMsgGCPIdentityFailed = "Failed to get GCP caller identity"

// errMsgGCPOrganizationFailed is a constant for the GCP organization error message.
const errMsgGCPOrganizationFailed = "Failed to get GCP organization info"

// getGCPIdentity is a helper that retrieves the GCP caller identity from the execution context.
func getGCPIdentity(ctx context.Context, execCtx *ExecutionContext) (*gcpIdentity.CallerIdentity, error) {
	defer perf.Track(nil, "function.getGCPIdentity")()

	// Get GCP auth context from stack info if available.
	var gcpAuth *schema.GCPAuthContext
	if execCtx != nil && execCtx.StackInfo != nil &&
		execCtx.StackInfo.AuthContext != nil && execCtx.StackInfo.AuthContext.GCP != nil {
		gcpAuth = execCtx.StackInfo.AuthContext.GCP
	}

	// Get AtmosConfig from execution context.
	var atmosConfig *schema.AtmosConfiguration
	if execCtx != nil {
		atmosConfig = execCtx.AtmosConfig
	}

	// Get the GCP caller identity (cached).
	return gcpIdentity.GetCallerIdentityCached(ctx, atmosConfig, gcpAuth)
}

// getGCPOrganization is a helper that retrieves the GCP organization info from the execution context.
func getGCPOrganization(ctx context.Context, execCtx *ExecutionContext) (*gcpOrg.OrganizationInfo, error) {
	defer perf.Track(nil, "function.getGCPOrganization")()

	// Get GCP auth context from stack info if available.
	var gcpAuth *schema.GCPAuthContext
	if execCtx != nil && execCtx.StackInfo != nil &&
		execCtx.StackInfo.AuthContext != nil && execCtx.StackInfo.AuthContext.GCP != nil {
		gcpAuth = execCtx.StackInfo.AuthContext.GCP
	}

	// Get AtmosConfig from execution context.
	var atmosConfig *schema.AtmosConfiguration
	if execCtx != nil {
		atmosConfig = execCtx.AtmosConfig
	}

	// Get the GCP organization info (cached).
	return gcpOrg.GetOrganizationCached(ctx, atmosConfig, gcpAuth)
}

// GcpProjectIDFunction implements the gcp.project_id function.
type GcpProjectIDFunction struct {
	BaseFunction
}

// NewGcpProjectIDFunction creates a new gcp.project_id function handler.
func NewGcpProjectIDFunction() *GcpProjectIDFunction {
	defer perf.Track(nil, "function.NewGcpProjectIDFunction")()

	return &GcpProjectIDFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagGcpProjectID,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the gcp.project_id function.
// Usage:
//
//	!gcp.project_id   - Returns the GCP project ID of the current caller identity
func (f *GcpProjectIDFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GcpProjectIDFunction.Execute")()

	log.Debug("Executing gcp.project_id function")

	identity, err := getGCPIdentity(ctx, execCtx)
	if err != nil {
		log.Error(errMsgGCPIdentityFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !gcp.project_id", "project_id", identity.ProjectID)
	return identity.ProjectID, nil
}

// GcpServiceAccountEmailFunction implements the gcp.service_account_email function.
type GcpServiceAccountEmailFunction struct {
	BaseFunction
}

// NewGcpServiceAccountEmailFunction creates a new gcp.service_account_email function handler.
func NewGcpServiceAccountEmailFunction() *GcpServiceAccountEmailFunction {
	defer perf.Track(nil, "function.NewGcpServiceAccountEmailFunction")()

	return &GcpServiceAccountEmailFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagGcpServiceAccountEmail,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the gcp.service_account_email function.
// Usage:
//
//	!gcp.service_account_email   - Returns the GCP service account email of the current caller
func (f *GcpServiceAccountEmailFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GcpServiceAccountEmailFunction.Execute")()

	log.Debug("Executing gcp.service_account_email function")

	identity, err := getGCPIdentity(ctx, execCtx)
	if err != nil {
		log.Error(errMsgGCPIdentityFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !gcp.service_account_email", "email", identity.ServiceAccountEmail)
	return identity.ServiceAccountEmail, nil
}

// GcpOrganizationIDFunction implements the gcp.organization_id function.
type GcpOrganizationIDFunction struct {
	BaseFunction
}

// NewGcpOrganizationIDFunction creates a new gcp.organization_id function handler.
func NewGcpOrganizationIDFunction() *GcpOrganizationIDFunction {
	defer perf.Track(nil, "function.NewGcpOrganizationIDFunction")()

	return &GcpOrganizationIDFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagGcpOrganizationID,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the gcp.organization_id function.
// Usage:
//
//	!gcp.organization_id   - Returns the GCP organization ID for the current project
func (f *GcpOrganizationIDFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GcpOrganizationIDFunction.Execute")()

	log.Debug("Executing gcp.organization_id function")

	orgInfo, err := getGCPOrganization(ctx, execCtx)
	if err != nil {
		log.Error(errMsgGCPOrganizationFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !gcp.organization_id", "organization_id", orgInfo.ID)
	return orgInfo.ID, nil
}

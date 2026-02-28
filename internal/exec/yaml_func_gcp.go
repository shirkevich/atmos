package exec

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const failedGetGCPIdentity = "Failed to get GCP caller identity"

// processTagGcpValue is a shared helper for GCP identity YAML functions.
// It validates the input tag, retrieves GCP caller identity, and returns the requested value.
func processTagGcpValue(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	expectedTag string,
	stackInfo *schema.ConfigAndStacksInfo,
	extractor func(*GCPCallerIdentity) string,
) any {
	log.Debug(execAWSYAMLFunction, functionKey, input)

	// Validate the tag matches expected.
	if input != expectedTag {
		log.Error(invalidYAMLFunction, functionKey, input, "expected", expectedTag)
		errUtils.CheckErrorPrintAndExit(errUtils.ErrYamlFuncInvalidArguments, "", "")
		return nil
	}

	// Get auth context from stack info if available.
	var authContext *schema.GCPAuthContext
	if stackInfo != nil && stackInfo.AuthContext != nil && stackInfo.AuthContext.GCP != nil {
		authContext = stackInfo.AuthContext.GCP
	}

	// Get the GCP caller identity (cached).
	ctx := context.Background()
	identity, err := getGCPCallerIdentityCached(ctx, atmosConfig, authContext)
	if err != nil {
		log.Error(failedGetGCPIdentity, "error", err)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Extract the requested value.
	return extractor(identity)
}

// processTagGcpProjectID processes the !gcp.project_id YAML function.
// It returns the GCP project ID from the current credentials or configuration.
// The function takes no parameters.
//
// Usage in YAML:
//
//	project_id: !gcp.project_id
func processTagGcpProjectID(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagGcpProjectID")()

	result := processTagGcpValue(atmosConfig, input, u.AtmosYamlFuncGcpProjectID, stackInfo, func(id *GCPCallerIdentity) string {
		return id.ProjectID
	})

	if result != nil {
		log.Debug("Resolved !gcp.project_id", "project_id", result)
	}
	return result
}

// processTagGcpServiceAccountEmail processes the !gcp.service_account_email YAML function.
// It returns the service account email from the current GCP credentials.
// The function takes no parameters.
//
// Usage in YAML:
//
//	service_account: !gcp.service_account_email
func processTagGcpServiceAccountEmail(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagGcpServiceAccountEmail")()

	result := processTagGcpValue(atmosConfig, input, u.AtmosYamlFuncGcpServiceAccountEmail, stackInfo, func(id *GCPCallerIdentity) string {
		return id.ServiceAccountEmail
	})

	if result != nil {
		log.Debug("Resolved !gcp.service_account_email", "service_account_email", result)
	}
	return result
}

// processTagGcpOrganizationID processes the !gcp.organization_id YAML function.
// It returns the GCP organization ID by walking the project's resource ancestry.
// The function takes no parameters.
//
// Usage in YAML:
//
//	org_id: !gcp.organization_id
func processTagGcpOrganizationID(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagGcpOrganizationID")()

	log.Debug(execAWSYAMLFunction, functionKey, input)

	// Validate the tag matches expected.
	if input != u.AtmosYamlFuncGcpOrganizationID {
		log.Error(invalidYAMLFunction, functionKey, input, "expected", u.AtmosYamlFuncGcpOrganizationID)
		errUtils.CheckErrorPrintAndExit(errUtils.ErrYamlFuncInvalidArguments, "", "")
		return nil
	}

	// Get auth context from stack info if available.
	var authContext *schema.GCPAuthContext
	if stackInfo != nil && stackInfo.AuthContext != nil && stackInfo.AuthContext.GCP != nil {
		authContext = stackInfo.AuthContext.GCP
	}

	// Get the GCP organization info (cached).
	ctx := context.Background()
	orgInfo, err := getGCPOrganizationCached(ctx, atmosConfig, authContext)
	if err != nil {
		log.Error("Failed to get GCP organization info", "error", err)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	if orgInfo == nil || orgInfo.ID == "" {
		log.Error("Failed to get GCP organization info", "error", errUtils.ErrGCPDescribeOrganization)
		errUtils.CheckErrorPrintAndExit(errUtils.ErrGCPDescribeOrganization, "", "")
		return nil
	}

	log.Debug("Resolved !gcp.organization_id", "organization_id", orgInfo.ID)
	return orgInfo.ID
}

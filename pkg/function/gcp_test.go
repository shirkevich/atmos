package function

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	gcpIdentity "github.com/cloudposse/atmos/pkg/gcp/identity"
	gcpOrg "github.com/cloudposse/atmos/pkg/gcp/organization"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGcpProjectIDFunction_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := gcpIdentity.NewMockGetter(ctrl)
	restore := gcpIdentity.SetGetter(mockGetter)
	defer restore()
	defer gcpIdentity.ClearIdentityCache()

	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	execCtx := &ExecutionContext{
		StackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{GCP: gcpAuth},
		},
	}

	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gcpAuth).
		Return(&gcpIdentity.CallerIdentity{ProjectID: "my-project"}, nil)

	fn := NewGcpProjectIDFunction()
	result, err := fn.Execute(context.Background(), "", execCtx)
	require.NoError(t, err)
	assert.Equal(t, "my-project", result)
}

func TestGcpServiceAccountEmailFunction_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := gcpIdentity.NewMockGetter(ctrl)
	restore := gcpIdentity.SetGetter(mockGetter)
	defer restore()
	defer gcpIdentity.ClearIdentityCache()

	gcpAuth := &schema.GCPAuthContext{
		ProjectID:           "my-project",
		ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
	}
	execCtx := &ExecutionContext{
		StackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{GCP: gcpAuth},
		},
	}

	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gcpAuth).
		Return(&gcpIdentity.CallerIdentity{
			ProjectID:           "my-project",
			ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
		}, nil)

	fn := NewGcpServiceAccountEmailFunction()
	result, err := fn.Execute(context.Background(), "", execCtx)
	require.NoError(t, err)
	assert.Equal(t, "sa@my-project.iam.gserviceaccount.com", result)
}

func TestGcpOrganizationIDFunction_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrgGetter := gcpOrg.NewMockGetter(ctrl)
	restoreOrg := gcpOrg.SetGetter(mockOrgGetter)
	defer restoreOrg()
	defer gcpOrg.ClearOrganizationCache()

	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	execCtx := &ExecutionContext{
		StackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{GCP: gcpAuth},
		},
	}

	mockOrgGetter.EXPECT().
		GetOrganization(gomock.Any(), gomock.Nil(), gcpAuth).
		Return(&gcpOrg.OrganizationInfo{
			ID:   "123456789",
			Name: "organizations/123456789",
		}, nil)

	fn := NewGcpOrganizationIDFunction()
	result, err := fn.Execute(context.Background(), "", execCtx)
	require.NoError(t, err)
	assert.Equal(t, "123456789", result)
}

func TestGcpProjectIDFunction_Execute_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := gcpIdentity.NewMockGetter(ctrl)
	restore := gcpIdentity.SetGetter(mockGetter)
	defer restore()
	defer gcpIdentity.ClearIdentityCache()

	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gomock.Nil()).
		Return(nil, errors.New("no credentials"))

	fn := NewGcpProjectIDFunction()
	_, err := fn.Execute(context.Background(), "", nil)
	assert.Error(t, err)
}

func TestGcpServiceAccountEmailFunction_Execute_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := gcpIdentity.NewMockGetter(ctrl)
	restore := gcpIdentity.SetGetter(mockGetter)
	defer restore()
	defer gcpIdentity.ClearIdentityCache()

	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gomock.Nil()).
		Return(nil, errors.New("no credentials"))

	fn := NewGcpServiceAccountEmailFunction()
	_, err := fn.Execute(context.Background(), "", nil)
	assert.Error(t, err)
}

func TestGcpOrganizationIDFunction_Execute_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrgGetter := gcpOrg.NewMockGetter(ctrl)
	restoreOrg := gcpOrg.SetGetter(mockOrgGetter)
	defer restoreOrg()
	defer gcpOrg.ClearOrganizationCache()

	mockOrgGetter.EXPECT().
		GetOrganization(gomock.Any(), gomock.Nil(), gomock.Nil()).
		Return(nil, errors.New("no credentials"))

	fn := NewGcpOrganizationIDFunction()
	_, err := fn.Execute(context.Background(), "", nil)
	assert.Error(t, err)
}

func TestGcpFunctions_Phase(t *testing.T) {
	assert.Equal(t, PostMerge, NewGcpProjectIDFunction().Phase())
	assert.Equal(t, PostMerge, NewGcpServiceAccountEmailFunction().Phase())
	assert.Equal(t, PostMerge, NewGcpOrganizationIDFunction().Phase())
}

func TestGcpFunctions_Names(t *testing.T) {
	assert.Equal(t, TagGcpProjectID, NewGcpProjectIDFunction().Name())
	assert.Equal(t, TagGcpServiceAccountEmail, NewGcpServiceAccountEmailFunction().Name())
	assert.Equal(t, TagGcpOrganizationID, NewGcpOrganizationIDFunction().Name())
}

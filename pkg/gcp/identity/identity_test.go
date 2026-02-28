package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetCallerIdentityCached_FromAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := NewMockGetter(ctrl)
	restore := SetGetter(mockGetter)
	defer restore()
	defer ClearIdentityCache()

	gcpAuth := &schema.GCPAuthContext{
		ProjectID:           "my-project",
		Region:              "us-central1",
		ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
	}

	expected := &CallerIdentity{
		ProjectID:           "my-project",
		Region:              "us-central1",
		ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
	}

	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gcpAuth).
		Return(expected, nil).
		Times(1)

	identity, err := GetCallerIdentityCached(context.Background(), nil, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, expected.ProjectID, identity.ProjectID)
	assert.Equal(t, expected.Region, identity.Region)
	assert.Equal(t, expected.ServiceAccountEmail, identity.ServiceAccountEmail)
}

func TestGetCallerIdentityCached_CachesResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := NewMockGetter(ctrl)
	restore := SetGetter(mockGetter)
	defer restore()
	defer ClearIdentityCache()

	gcpAuth := &schema.GCPAuthContext{
		ProjectID: "cached-project",
		Region:    "europe-west1",
	}

	expected := &CallerIdentity{
		ProjectID: "cached-project",
		Region:    "europe-west1",
	}

	// Should only be called once despite two calls to GetCallerIdentityCached.
	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gcpAuth).
		Return(expected, nil).
		Times(1)

	identity1, err := GetCallerIdentityCached(context.Background(), nil, gcpAuth)
	require.NoError(t, err)

	identity2, err := GetCallerIdentityCached(context.Background(), nil, gcpAuth)
	require.NoError(t, err)

	assert.Equal(t, identity1, identity2)
}

func TestGetCallerIdentityCached_CachesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := NewMockGetter(ctrl)
	restore := SetGetter(mockGetter)
	defer restore()
	defer ClearIdentityCache()

	expectedErr := errors.New("GCP unavailable")

	// Should only be called once; error is cached on second call.
	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gomock.Nil()).
		Return(nil, expectedErr).
		Times(1)

	_, err1 := GetCallerIdentityCached(context.Background(), nil, nil)
	assert.Error(t, err1)

	_, err2 := GetCallerIdentityCached(context.Background(), nil, nil)
	assert.Error(t, err2)
}

func TestClearIdentityCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGetter := NewMockGetter(ctrl)
	restore := SetGetter(mockGetter)
	defer restore()
	defer ClearIdentityCache()

	gcpAuth := &schema.GCPAuthContext{
		ProjectID: "clear-test-project",
	}

	expected := &CallerIdentity{ProjectID: "clear-test-project"}

	// Called twice because we clear the cache between calls.
	mockGetter.EXPECT().
		GetCallerIdentity(gomock.Any(), gomock.Nil(), gcpAuth).
		Return(expected, nil).
		Times(2)

	_, err := GetCallerIdentityCached(context.Background(), nil, gcpAuth)
	require.NoError(t, err)

	ClearIdentityCache()

	_, err = GetCallerIdentityCached(context.Background(), nil, gcpAuth)
	require.NoError(t, err)
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		gcpAuth  *schema.GCPAuthContext
		expected string
	}{
		{
			name:     "nil auth context returns default",
			gcpAuth:  nil,
			expected: "default",
		},
		{
			name: "auth context with values",
			gcpAuth: &schema.GCPAuthContext{
				ProjectID: "proj",
				Region:    "us-east1",
			},
			expected: "proj|us-east1|",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := getCacheKey(tc.gcpAuth)
			assert.Equal(t, tc.expected, key)
		})
	}
}

func TestGetProjectFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "GOOGLE_CLOUD_PROJECT takes priority",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "proj-1",
				"GCLOUD_PROJECT":       "proj-2",
				"CLOUDSDK_CORE_PROJECT": "",
			},
			expected: "proj-1",
		},
		{
			name: "GCLOUD_PROJECT as fallback",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "",
				"GCLOUD_PROJECT":       "proj-2",
				"CLOUDSDK_CORE_PROJECT": "",
			},
			expected: "proj-2",
		},
		{
			name: "CLOUDSDK_CORE_PROJECT as last fallback",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "",
				"GCLOUD_PROJECT":       "",
				"CLOUDSDK_CORE_PROJECT": "proj-3",
			},
			expected: "proj-3",
		},
		{
			name: "empty when none set",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "",
				"GCLOUD_PROJECT":       "",
				"CLOUDSDK_CORE_PROJECT": "",
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.expected, getProjectFromEnv())
		})
	}
}

func TestGetRegionFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "GOOGLE_CLOUD_REGION takes priority",
			envVars: map[string]string{
				"GOOGLE_CLOUD_REGION":    "us-central1",
				"CLOUDSDK_COMPUTE_REGION": "us-east1",
			},
			expected: "us-central1",
		},
		{
			name: "CLOUDSDK_COMPUTE_REGION as fallback",
			envVars: map[string]string{
				"GOOGLE_CLOUD_REGION":    "",
				"CLOUDSDK_COMPUTE_REGION": "us-east1",
			},
			expected: "us-east1",
		},
		{
			name: "empty when none set",
			envVars: map[string]string{
				"GOOGLE_CLOUD_REGION":    "",
				"CLOUDSDK_COMPUTE_REGION": "",
			},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.expected, getRegionFromEnv())
		})
	}
}

// TestDefaultGetter_GetCallerIdentity_FromAuthContext tests that a complete GCPAuthContext
// is used directly without any env var or ADC lookup.
func TestDefaultGetter_GetCallerIdentity_FromAuthContext(t *testing.T) {
	gcpAuth := &schema.GCPAuthContext{
		ProjectID:           "auth-project",
		Region:              "europe-west1",
		ServiceAccountEmail: "sa@auth-project.iam.gserviceaccount.com",
	}

	g := &defaultGetter{}
	identity, err := g.GetCallerIdentity(context.Background(), nil, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "auth-project", identity.ProjectID)
	assert.Equal(t, "europe-west1", identity.Region)
	assert.Equal(t, "sa@auth-project.iam.gserviceaccount.com", identity.ServiceAccountEmail)
}

// TestDefaultGetter_GetCallerIdentity_ADCFallback tests that ADC failure is non-fatal.
// When gcpAuth provides project and region but no service account email, ADC is attempted.
// In environments without GCP credentials, ADC fails but the function succeeds with partial data.
func TestDefaultGetter_GetCallerIdentity_ADCFallback(t *testing.T) {
	gcpAuth := &schema.GCPAuthContext{
		ProjectID: "fallback-project",
		Region:    "us-west2",
		// ServiceAccountEmail intentionally empty to trigger ADC lookup.
	}

	g := &defaultGetter{}
	// ADC will fail in test environments without GCP credentials (non-fatal).
	identity, err := g.GetCallerIdentity(context.Background(), nil, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "fallback-project", identity.ProjectID)
	assert.Equal(t, "us-west2", identity.Region)
	// ServiceAccountEmail may be set from ADC or remain empty depending on environment.
}

// TestDefaultGetter_GetCallerIdentity_NoProjectID tests that an error is returned when no project ID
// is available from any source and ADC is not configured.
// This test skips gracefully in environments where ADC provides a project ID.
func TestDefaultGetter_GetCallerIdentity_NoProjectID(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("CLOUDSDK_CORE_PROJECT", "")

	g := &defaultGetter{}
	_, err := g.GetCallerIdentity(context.Background(), nil, nil)
	if err != nil {
		// Expected in environments without GCP ADC configured.
		assert.ErrorIs(t, err, errUtils.ErrGCPGetCallerIdentity)
	}
	// If err is nil, ADC provided a project ID (e.g., in GCP Cloud Shell). That is also valid.
}

// TestDefaultGetter_GetCallerIdentity_FromEnv tests the env var fallback path.
// ADC may be attempted (non-fatal failure) since service account email is not in env vars.
func TestDefaultGetter_GetCallerIdentity_FromEnv(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")
	t.Setenv("GOOGLE_CLOUD_REGION", "us-east1")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("CLOUDSDK_CORE_PROJECT", "")
	t.Setenv("CLOUDSDK_COMPUTE_REGION", "")

	g := &defaultGetter{}
	// ADC will fail in test environments without GCP credentials (non-fatal).
	identity, err := g.GetCallerIdentity(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "env-project", identity.ProjectID)
	assert.Equal(t, "us-east1", identity.Region)
}

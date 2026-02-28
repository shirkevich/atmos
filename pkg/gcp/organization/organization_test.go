package organization

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/cloudresourcemanager/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withMockCRMClient replaces the newCRMClient factory for testing defaultGetter.
// It returns a cleanup function to restore the original factory.
func withMockCRMClient(t *testing.T, mockSvc crmProjectsService, factoryErr error) func() {
	t.Helper()

	oldFactory := newCRMClient
	newCRMClient = func(_ context.Context, _ *schema.GCPAuthContext) (crmProjectsService, error) {
		if factoryErr != nil {
			return nil, factoryErr
		}
		return mockSvc, nil
	}
	return func() {
		newCRMClient = oldFactory
	}
}

func TestGetOrganizationCached_Success(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedInfo := &OrganizationInfo{
		ID:   "123456789",
		Name: "organizations/123456789",
	}

	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedInfo, nil).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}

	info, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "123456789", info.ID)
	assert.Equal(t, "organizations/123456789", info.Name)
}

func TestGetOrganizationCached_CacheBehavior(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedInfo := &OrganizationInfo{
		ID:   "999888777",
		Name: "organizations/999888777",
	}

	// Expect exactly 2 calls: first call, and call after cache clear.
	// The different-project call is a 3rd unique cache key.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedInfo, nil).
		Times(3)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "project-a"}

	// First call should hit the mock.
	info1, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "999888777", info1.ID)

	// Second call with same auth context should use cache (no additional mock call).
	info2, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "999888777", info2.ID)

	// Call with different project should call getter again.
	differentAuth := &schema.GCPAuthContext{ProjectID: "project-b"}
	info3, err := GetOrganizationCached(ctx, atmosConfig, differentAuth)
	require.NoError(t, err)
	assert.Equal(t, "999888777", info3.ID)

	// Clear cache and verify next call hits mock.
	ClearOrganizationCache()
	info4, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "999888777", info4.ID)
}

func TestGetOrganizationCached_ErrorCaching(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedErr := errors.New("mock organization error")

	// Expect exactly 1 call; second call should use cached error.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, expectedErr).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}

	// First call should return error and cache it.
	_, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.Error(t, err)

	// Second call should return cached error (no additional mock call).
	_, err = GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.Error(t, err)
}

func TestGetOrganizationCached_Concurrent(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	expectedInfo := &OrganizationInfo{
		ID:   "111222333",
		Name: "organizations/111222333",
	}

	// Despite many goroutines, expect at most 1 call due to caching.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(expectedInfo, nil).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "concurrent-project"}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			info, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
			assert.NoError(t, err)
			assert.Equal(t, "111222333", info.ID)
		}()
	}

	wg.Wait()
}

func TestSetGetter_Restore(t *testing.T) {
	ClearOrganizationCache()

	originalGetter := getter

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	restore := SetGetter(mock)

	// Verify mock is active.
	assert.Equal(t, mock, getter)

	// Restore original.
	restore()

	// Verify original is restored.
	assert.Equal(t, originalGetter, getter)
}

func TestSetGetter_NilFallback(t *testing.T) {
	originalGetter := getter
	defer func() { getter = originalGetter }()

	// Calling SetGetter(nil) should reset getter to defaultGetter, not set it to nil.
	SetGetter(nil)
	assert.IsType(t, &defaultGetter{}, getter)
	assert.NotNil(t, getter)
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		gcpAuth  *schema.GCPAuthContext
		expected string
	}{
		{
			name:     "nil auth context",
			gcpAuth:  nil,
			expected: "default",
		},
		{
			name:     "empty project ID",
			gcpAuth:  &schema.GCPAuthContext{},
			expected: "default",
		},
		{
			name:     "with project ID",
			gcpAuth:  &schema.GCPAuthContext{ProjectID: "my-project-123"},
			expected: "my-project-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCacheKey(tt.gcpAuth)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetOrganizationCached_DoubleCheckHit(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	// Expect zero calls because we pre-populate the cache.
	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "pre-populated-project"}

	// Pre-populate the cache directly to simulate another goroutine having cached the result
	// between our read-lock miss and write-lock acquisition (the double-check branch).
	cacheKey := getCacheKey(gcpAuth)
	organizationCacheMu.Lock()
	organizationCache[cacheKey] = &cachedOrganization{
		info: &OrganizationInfo{ID: "pre-populated-org"},
		err:  nil,
	}
	organizationCacheMu.Unlock()

	// This call should hit the double-check branch and return the pre-populated value.
	info, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "pre-populated-org", info.ID)
}

func TestClearOrganizationCache(t *testing.T) {
	ClearOrganizationCache()

	ctrl := gomock.NewController(t)
	mock := NewMockGetter(ctrl)

	mock.EXPECT().
		GetOrganization(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&OrganizationInfo{ID: "clear-test-org"}, nil).
		Times(1)

	restore := SetGetter(mock)
	defer restore()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "clear-test-project"}

	// Populate cache.
	_, err := GetOrganizationCached(ctx, atmosConfig, gcpAuth)
	require.NoError(t, err)

	// Clear cache.
	ClearOrganizationCache()

	// Verify cache is empty by checking the internal map.
	organizationCacheMu.RLock()
	assert.Empty(t, organizationCache)
	organizationCacheMu.RUnlock()
}

func TestDefaultGetter_GetOrganization_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	mockSvc.EXPECT().
		GetAncestry("my-project", gomock.Any()).
		Return(&cloudresourcemanager.GetAncestryResponse{
			Ancestor: []*cloudresourcemanager.Ancestor{
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "project", Id: "my-project"}},
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "folder", Id: "456"}},
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "organization", Id: "123456789"}},
			},
		}, nil).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.NoError(t, err)
	assert.Equal(t, "123456789", info.ID)
	assert.Equal(t, "organizations/123456789", info.Name)
}

func TestDefaultGetter_GetOrganization_EmptyProjectID(t *testing.T) {
	d := &defaultGetter{}
	// No project ID in auth context and GOOGLE_CLOUD_PROJECT not set.
	gcpAuth := &schema.GCPAuthContext{}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGCPProjectIDRequired)
}

func TestDefaultGetter_GetOrganization_NilAuth(t *testing.T) {
	d := &defaultGetter{}
	// Nil auth context - should still fail if no env var set.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGCPProjectIDRequired)
}

func TestDefaultGetter_GetOrganization_ClientFactoryError(t *testing.T) {
	factoryErr := errors.New("failed to create CRM client")
	restore := withMockCRMClient(t, nil, factoryErr)
	defer restore()

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGCPDescribeOrganization)
	assert.ErrorIs(t, err, factoryErr)
}

func TestDefaultGetter_GetOrganization_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	apiErr := errors.New("permission denied")
	mockSvc.EXPECT().
		GetAncestry("my-project", gomock.Any()).
		Return(nil, apiErr).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGCPDescribeOrganization)
	assert.ErrorIs(t, err, apiErr)
}

func TestDefaultGetter_GetOrganization_NoOrgAncestor(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	// Personal project ancestry has no organization ancestor.
	mockSvc.EXPECT().
		GetAncestry("personal-project", gomock.Any()).
		Return(&cloudresourcemanager.GetAncestryResponse{
			Ancestor: []*cloudresourcemanager.Ancestor{
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "project", Id: "personal-project"}},
			},
		}, nil).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "personal-project"}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGCPDescribeOrganization)
	assert.Contains(t, err.Error(), "no organization ancestor")
}

func TestDefaultGetter_GetOrganization_NilResourceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	// Ancestry contains a nil ResourceId entry, which should be skipped.
	mockSvc.EXPECT().
		GetAncestry("my-project", gomock.Any()).
		Return(&cloudresourcemanager.GetAncestryResponse{
			Ancestor: []*cloudresourcemanager.Ancestor{
				{ResourceId: nil},
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "organization", Id: "987654321"}},
			},
		}, nil).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.NoError(t, err)
	assert.Equal(t, "987654321", info.ID)
	assert.Equal(t, "organizations/987654321", info.Name)
}

func TestDefaultGetter_GetOrganization_EnvVarProjectID(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	mockSvc.EXPECT().
		GetAncestry("env-project", gomock.Any()).
		Return(&cloudresourcemanager.GetAncestryResponse{
			Ancestor: []*cloudresourcemanager.Ancestor{
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "project", Id: "env-project"}},
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "organization", Id: "555666777"}},
			},
		}, nil).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")

	d := &defaultGetter{}
	// No project ID in gcpAuth - should use env var.
	gcpAuth := &schema.GCPAuthContext{}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.NoError(t, err)
	assert.Equal(t, "555666777", info.ID)
}

func TestGetProjectID(t *testing.T) {
	tests := []struct {
		name     string
		gcpAuth  *schema.GCPAuthContext
		envVar   string
		expected string
	}{
		{
			name:     "nil auth context uses env var",
			gcpAuth:  nil,
			envVar:   "from-env",
			expected: "from-env",
		},
		{
			name:     "auth context project ID takes precedence over env var",
			gcpAuth:  &schema.GCPAuthContext{ProjectID: "from-auth"},
			envVar:   "from-env",
			expected: "from-auth",
		},
		{
			name:     "empty auth context falls back to env var",
			gcpAuth:  &schema.GCPAuthContext{},
			envVar:   "from-env",
			expected: "from-env",
		},
		{
			name:     "empty auth context and no env var returns empty string",
			gcpAuth:  &schema.GCPAuthContext{},
			envVar:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_PROJECT", tt.envVar)
			result := getProjectID(tt.gcpAuth)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildClientOptions_AccessToken(t *testing.T) {
	gcpAuth := &schema.GCPAuthContext{
		ProjectID:   "my-project",
		AccessToken: "ya29.test-token",
	}
	opts := buildClientOptions(gcpAuth)
	// Should return exactly one option (the token source).
	assert.Len(t, opts, 1)
}

func TestBuildClientOptions_NoAuth(t *testing.T) {
	gcpAuth := &schema.GCPAuthContext{ProjectID: "my-project"}
	opts := buildClientOptions(gcpAuth)
	// No explicit auth - should return empty options (ADC will be used).
	assert.Empty(t, opts)
}

func TestBuildClientOptions_NilAuth(t *testing.T) {
	opts := buildClientOptions(nil)
	// Nil auth - should return empty options (ADC will be used).
	assert.Empty(t, opts)
}

func TestDefaultGetter_GetOrganization_ProjectIDPrecedence(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	// The auth context project ID should take precedence over env var.
	mockSvc.EXPECT().
		GetAncestry("auth-project", gomock.Any()).
		Return(&cloudresourcemanager.GetAncestryResponse{
			Ancestor: []*cloudresourcemanager.Ancestor{
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "organization", Id: "100200300"}},
			},
		}, nil).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "auth-project"}
	info, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.NoError(t, err)
	assert.Equal(t, "100200300", info.ID)
}

func TestDefaultGetter_GetOrganization_ErrorMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSvc := NewMockcrmProjectsService(ctrl)

	mockSvc.EXPECT().
		GetAncestry("standalone-project", gomock.Any()).
		Return(&cloudresourcemanager.GetAncestryResponse{
			Ancestor: []*cloudresourcemanager.Ancestor{
				{ResourceId: &cloudresourcemanager.ResourceId{Type: "project", Id: "standalone-project"}},
			},
		}, nil).
		Times(1)

	restore := withMockCRMClient(t, mockSvc, nil)
	defer restore()

	d := &defaultGetter{}
	gcpAuth := &schema.GCPAuthContext{ProjectID: "standalone-project"}
	_, err := d.GetOrganization(context.Background(), &schema.AtmosConfiguration{}, gcpAuth)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "standalone-project")
	assert.Contains(t, err.Error(), fmt.Sprintf("%s", errUtils.ErrGCPDescribeOrganization))
}

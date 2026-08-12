package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type contentModerationScopeAccountRepo struct {
	accounts map[int64]*Account
	err      error
	calls    int
}

type contentModerationScopeGroupRepo struct {
	GroupRepository
	accountIDs []int64
	err        error
}

func (r *contentModerationScopeGroupRepo) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]int64(nil), r.accountIDs...), nil
}

func contentModerationScopeInt64Ptr(value int64) *int64 {
	return &value
}

func (r *contentModerationScopeAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	accounts := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account := r.accounts[id]; account != nil {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func TestContentModerationScopeFiltersNormalizeAndPersist(t *testing.T) {
	repo := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	userFilter := ContentModerationUserFilter{
		Type:    " INCLUDE ",
		UserIDs: []int64{9, 2, 9, 0, -1},
	}
	accountFilter := ContentModerationAccountFilter{
		Type:       "Exclude",
		AccountIDs: []int64{88, 4, 88, 0},
	}

	view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
		UserFilter:    &userFilter,
		AccountFilter: &accountFilter,
	})
	require.NoError(t, err)
	require.Equal(t, ContentModerationScopeFilterInclude, view.UserFilter.Type)
	require.Equal(t, []int64{2, 9}, view.UserFilter.UserIDs)
	require.Equal(t, ContentModerationScopeFilterExclude, view.AccountFilter.Type)
	require.Equal(t, []int64{4, 88}, view.AccountFilter.AccountIDs)

	var saved ContentModerationConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyContentModerationConfig]), &saved))
	require.Equal(t, view.UserFilter, saved.UserFilter)
	require.Equal(t, view.AccountFilter, saved.AccountFilter)
}

func TestContentModerationUpdateConfigCanonicalizesSparkShadowAccountIDs(t *testing.T) {
	const (
		parentID  int64 = 41
		shadowID  int64 = 141
		missingID int64 = 999
	)
	tests := []struct {
		name          string
		filterType    string
		accountIDs    []int64
		wantIDs       []int64
		wantRepoCalls int
	}{
		{name: "include", filterType: ContentModerationScopeFilterInclude, accountIDs: []int64{shadowID, parentID, missingID}, wantIDs: []int64{parentID, missingID}, wantRepoCalls: 1},
		{name: "exclude", filterType: ContentModerationScopeFilterExclude, accountIDs: []int64{shadowID, parentID, missingID}, wantIDs: []int64{parentID, missingID}, wantRepoCalls: 1},
		{name: "all", filterType: ContentModerationScopeFilterAll, accountIDs: []int64{shadowID}, wantIDs: []int64{}, wantRepoCalls: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := &contentModerationTestSettingRepo{values: map[string]string{}}
			accountRepo := &contentModerationScopeAccountRepo{accounts: map[int64]*Account{
				parentID: {ID: parentID},
				shadowID: {ID: shadowID, ParentAccountID: contentModerationScopeInt64Ptr(parentID)},
			}}
			svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, nil, nil, nil)
			svc.accountScopeRepo = accountRepo
			filter := ContentModerationAccountFilter{Type: tt.filterType, AccountIDs: tt.accountIDs}

			view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AccountFilter: &filter})
			require.NoError(t, err)
			require.Equal(t, tt.filterType, view.AccountFilter.Type)
			require.Equal(t, tt.wantIDs, view.AccountFilter.AccountIDs)
			require.Equal(t, tt.wantRepoCalls, accountRepo.calls)

			var saved ContentModerationConfig
			require.NoError(t, json.Unmarshal([]byte(settingRepo.values[SettingKeyContentModerationConfig]), &saved))
			require.Equal(t, tt.wantIDs, saved.AccountFilter.AccountIDs)
		})
	}
}

func TestContentModerationLegacyShadowAccountScopeMatchesCanonicalParentAtRuntime(t *testing.T) {
	const (
		parentID int64 = 52
		shadowID int64 = 152
		otherID  int64 = 53
	)
	for _, filterType := range []string{ContentModerationScopeFilterInclude, ContentModerationScopeFilterExclude} {
		t.Run(filterType, func(t *testing.T) {
			cfg := defaultContentModerationConfig()
			cfg.AccountFilter = ContentModerationAccountFilter{Type: filterType, AccountIDs: []int64{shadowID}}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			settingRepo := &contentModerationTestSettingRepo{values: map[string]string{
				SettingKeyRiskControlEnabled:      "true",
				SettingKeyContentModerationConfig: string(raw),
			}}
			accountRepo := &contentModerationScopeAccountRepo{accounts: map[int64]*Account{
				shadowID: {ID: shadowID, ParentAccountID: contentModerationScopeInt64Ptr(parentID)},
			}}
			svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, nil, nil, nil)
			svc.accountScopeRepo = accountRepo

			view, err := svc.GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, []int64{parentID}, view.AccountFilter.AccountIDs)

			shouldAuditParent, _, err := svc.ShouldAuditAccount(context.Background(), parentID)
			require.NoError(t, err)
			shouldAuditOther, _, err := svc.ShouldAuditAccount(context.Background(), otherID)
			require.NoError(t, err)
			if filterType == ContentModerationScopeFilterInclude {
				require.True(t, shouldAuditParent)
				require.False(t, shouldAuditOther)
			} else {
				require.False(t, shouldAuditParent)
				require.True(t, shouldAuditOther)
			}
		})
	}
}

func TestContentModerationAccountScopeLookupFailureIsExplicit(t *testing.T) {
	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, nil, nil, nil)
	svc.accountScopeRepo = &contentModerationScopeAccountRepo{err: errors.New("database unavailable")}
	filter := ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{123}}

	_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AccountFilter: &filter})
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve content moderation account scope IDs")
	require.ErrorContains(t, err, "database unavailable")
}

func TestContentModerationRuntimeScopeLookupFailureFallsBackToAuditAllAccounts(t *testing.T) {
	const (
		parentID int64 = 23
		shadowID int64 = 123
	)
	cfg := defaultContentModerationConfig()
	cfg.AccountFilter = ContentModerationAccountFilter{
		Type:       ContentModerationScopeFilterInclude,
		AccountIDs: []int64{shadowID},
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: string(raw),
	}}
	accountRepo := &contentModerationScopeAccountRepo{
		accounts: map[int64]*Account{
			shadowID: {ID: shadowID, ParentAccountID: contentModerationScopeInt64Ptr(parentID)},
		},
		err: errors.New("database unavailable"),
	}
	svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, nil, nil, nil)
	svc.accountScopeRepo = accountRepo

	shouldAudit, reason, err := svc.ShouldAuditAccount(context.Background(), 999)
	require.NoError(t, err)
	require.True(t, shouldAudit)
	require.Empty(t, reason)
	require.Equal(t, 1, accountRepo.calls)

	snapshot := svc.runtimeSnapshot.Load()
	require.NotNil(t, snapshot)
	require.Equal(t, ContentModerationScopeFilterAll, snapshot.config.AccountFilter.Type)
	require.Empty(t, snapshot.config.AccountFilter.AccountIDs)
	require.True(t, snapshot.accountScopeFallback)

	accountRepo.err = nil
	snapshot, err = svc.refreshRuntimeSnapshot(context.Background())
	require.NoError(t, err)
	require.False(t, snapshot.accountScopeFallback)
	require.Equal(t, ContentModerationScopeFilterInclude, snapshot.config.AccountFilter.Type)
	require.Equal(t, []int64{parentID}, snapshot.config.AccountFilter.AccountIDs)
	require.Equal(t, 2, accountRepo.calls)

	shouldAudit, reason, err = svc.ShouldAuditAccount(context.Background(), parentID)
	require.NoError(t, err)
	require.True(t, shouldAudit)
	require.Empty(t, reason)
	shouldAudit, reason, err = svc.ShouldAuditAccount(context.Background(), 999)
	require.NoError(t, err)
	require.False(t, shouldAudit)
	require.Equal(t, ContentModerationScopeReasonAccountOutOfScope, reason)
}

func TestContentModerationScopeFiltersRejectInvalidTypes(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateContentModerationConfigInput
		code  string
	}{
		{
			name: "user filter",
			input: UpdateContentModerationConfigInput{UserFilter: &ContentModerationUserFilter{
				Type: "selected",
			}},
			code: "INVALID_CONTENT_MODERATION_USER_FILTER",
		},
		{
			name: "account filter",
			input: UpdateContentModerationConfigInput{AccountFilter: &ContentModerationAccountFilter{
				Type: "selected",
			}},
			code: "INVALID_CONTENT_MODERATION_ACCOUNT_FILTER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &contentModerationTestSettingRepo{values: map[string]string{}}
			svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
			_, err := svc.UpdateConfig(context.Background(), tt.input)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.code)
		})
	}
}

func TestContentModerationScopeMatchingHelpers(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.UserFilter = ContentModerationUserFilter{
		Type:    ContentModerationScopeFilterInclude,
		UserIDs: []int64{10},
	}
	cfg.AccountFilter = ContentModerationAccountFilter{
		Type:       ContentModerationScopeFilterExclude,
		AccountIDs: []int64{21},
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: string(raw),
	}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)

	shouldAudit, reason, err := svc.ShouldAuditUser(context.Background(), 10)
	require.NoError(t, err)
	require.True(t, shouldAudit)
	require.Empty(t, reason)

	shouldAudit, reason, err = svc.ShouldAuditUser(context.Background(), 11)
	require.NoError(t, err)
	require.False(t, shouldAudit)
	require.Equal(t, ContentModerationScopeReasonUserOutOfScope, reason)

	requiresAccount, err := svc.RequiresAccountScopeResolution(context.Background())
	require.NoError(t, err)
	require.True(t, requiresAccount)

	shouldAudit, reason, err = svc.ShouldAuditAccount(context.Background(), 21)
	require.NoError(t, err)
	require.False(t, shouldAudit)
	require.Equal(t, ContentModerationScopeReasonAccountOutOfScope, reason)

	shouldAudit, reason, err = svc.ShouldAuditAccount(context.Background(), 22)
	require.NoError(t, err)
	require.True(t, shouldAudit)
	require.Empty(t, reason)
}

func TestContentModerationCanAuditGroupBeforeAccountSelection(t *testing.T) {
	groupID := int64(8)
	tests := []struct {
		name       string
		filter     ContentModerationAccountFilter
		accountIDs []int64
		want       bool
	}{
		{name: "all", filter: ContentModerationAccountFilter{Type: ContentModerationScopeFilterAll}, accountIDs: []int64{21, 22}, want: true},
		{name: "include fully protected", filter: ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{21, 22}}, accountIDs: []int64{21, 22}, want: true},
		{name: "include mixed", filter: ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{21}}, accountIDs: []int64{21, 22}, want: false},
		{name: "exclude fully protected", filter: ContentModerationAccountFilter{Type: ContentModerationScopeFilterExclude, AccountIDs: []int64{23}}, accountIDs: []int64{21, 22}, want: true},
		{name: "exclude mixed", filter: ContentModerationAccountFilter{Type: ContentModerationScopeFilterExclude, AccountIDs: []int64{22}}, accountIDs: []int64{21, 22}, want: false},
		{name: "empty group", filter: ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{21}}, accountIDs: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := &contentModerationTestSettingRepo{values: map[string]string{}}
			groupRepo := &contentModerationScopeGroupRepo{accountIDs: tt.accountIDs}
			svc := NewContentModerationService(settingRepo, nil, nil, groupRepo, nil, nil, nil, nil)
			_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AccountFilter: &tt.filter})
			require.NoError(t, err)

			got, err := svc.CanAuditGroupBeforeAccountSelection(context.Background(), &groupID)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestContentModerationCanAuditGroupCanonicalizesShadowAccounts(t *testing.T) {
	const (
		parentID int64 = 21
		shadowID int64 = 121
	)
	groupID := int64(8)
	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{}}
	groupRepo := &contentModerationScopeGroupRepo{accountIDs: []int64{shadowID}}
	accountRepo := &contentModerationScopeAccountRepo{accounts: map[int64]*Account{
		shadowID: {ID: shadowID, ParentAccountID: contentModerationScopeInt64Ptr(parentID)},
	}}
	svc := NewContentModerationService(settingRepo, nil, nil, groupRepo, nil, nil, nil, nil)
	svc.accountScopeRepo = accountRepo
	filter := ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{parentID}}
	_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AccountFilter: &filter})
	require.NoError(t, err)

	got, err := svc.CanAuditGroupBeforeAccountSelection(context.Background(), &groupID)
	require.NoError(t, err)
	require.True(t, got)
}

func TestContentModerationCanAuditGroupLookupFailureIsExplicit(t *testing.T) {
	groupID := int64(8)
	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{}}
	groupRepo := &contentModerationScopeGroupRepo{err: errors.New("database unavailable")}
	svc := NewContentModerationService(settingRepo, nil, nil, groupRepo, nil, nil, nil, nil)
	filter := ContentModerationAccountFilter{Type: ContentModerationScopeFilterInclude, AccountIDs: []int64{21}}
	_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{AccountFilter: &filter})
	require.NoError(t, err)

	_, err = svc.CanAuditGroupBeforeAccountSelection(context.Background(), &groupID)
	require.ErrorContains(t, err, "resolve content moderation group accounts")
	require.ErrorContains(t, err, "database unavailable")
}

func TestContentModerationScopeFilterEmptyListSemantics(t *testing.T) {
	require.False(t, (&ContentModerationConfig{UserFilter: ContentModerationUserFilter{
		Type: ContentModerationScopeFilterInclude,
	}}).includesUser(1))
	require.True(t, (&ContentModerationConfig{UserFilter: ContentModerationUserFilter{
		Type: ContentModerationScopeFilterExclude,
	}}).includesUser(1))
	require.False(t, (&ContentModerationConfig{AccountFilter: ContentModerationAccountFilter{
		Type: ContentModerationScopeFilterInclude,
	}}).includesAccount(1))
	require.True(t, (&ContentModerationConfig{AccountFilter: ContentModerationAccountFilter{
		Type: ContentModerationScopeFilterExclude,
	}}).includesAccount(1))
}

func TestContentModerationLegacyConfigDefaultsScopeFiltersToAll(t *testing.T) {
	cfg, err := parseContentModerationConfig(`{"enabled":true}`)
	require.NoError(t, err)
	require.Equal(t, ContentModerationScopeFilterAll, cfg.UserFilter.Type)
	require.Empty(t, cfg.UserFilter.UserIDs)
	require.Equal(t, ContentModerationScopeFilterAll, cfg.AccountFilter.Type)
	require.Empty(t, cfg.AccountFilter.AccountIDs)

	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: string(raw),
	}}
	svc := NewContentModerationService(repo, nil, nil, nil, nil, nil, nil, nil)
	requiresAccount, err := svc.RequiresAccountScopeResolution(context.Background())
	require.NoError(t, err)
	require.False(t, requiresAccount)
}

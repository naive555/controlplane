package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/controlplane/backend/internal/infra/database/db"
	"github.com/controlplane/backend/internal/shared/apperror"
)

type mockSubStore struct {
	getOrgSubscriptionWithPlan func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error)
}

func (m *mockSubStore) GetOrgSubscriptionWithPlan(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
	return m.getOrgSubscriptionWithPlan(ctx, organizationID)
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func appErrCode(t *testing.T, err error) string {
	t.Helper()
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.Error, got %T (%v)", err, err)
	}
	return appErr.Code
}

func TestEnforceLimit_NoSubscriptionIsUnlimited(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{}, pgx.ErrNoRows
		},
	})

	if err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 1_000_000); err != nil {
		t.Fatalf("expected no error for an org with no subscription, got %v", err)
	}
}

func TestEnforceLimit_NegativeOneIsUnlimited(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{
				PlanLimits: mustMarshal(t, map[string]float64{"max_members": -1}),
			}, nil
		},
	})

	if err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 1_000_000); err != nil {
		t.Fatalf("expected no error for a -1 (enterprise) limit, got %v", err)
	}
}

func TestEnforceLimit_UnderLimitPasses(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{
				PlanLimits: mustMarshal(t, map[string]float64{"max_members": 5}),
			}, nil
		},
	})

	if err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 4); err != nil {
		t.Fatalf("expected no error when currentCount < limit, got %v", err)
	}
}

func TestEnforceLimit_AtLimitFails(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{
				PlanLimits: mustMarshal(t, map[string]float64{"max_members": 5}),
			}, nil
		},
	})

	err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 5)
	if code := appErrCode(t, err); code != apperror.LimitExceeded {
		t.Errorf("code = %q, want %q", code, apperror.LimitExceeded)
	}
}

func TestEnforceLimit_OverLimitFails(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{
				PlanLimits: mustMarshal(t, map[string]float64{"max_members": 5}),
			}, nil
		},
	})

	err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 6)
	if code := appErrCode(t, err); code != apperror.LimitExceeded {
		t.Errorf("code = %q, want %q", code, apperror.LimitExceeded)
	}
}

func TestEnforceLimit_CustomLimitsOverridePlan(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{
				PlanLimits:   mustMarshal(t, map[string]float64{"max_members": 5}),
				CustomLimits: mustMarshal(t, map[string]float64{"max_members": 100}),
			}, nil
		},
	})

	if err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 50); err != nil {
		t.Fatalf("expected custom_limits to override plan limits, got %v", err)
	}
}

func TestEnforceLimit_KeyMissingIsUnlimited(t *testing.T) {
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{
				PlanLimits: mustMarshal(t, map[string]float64{"max_projects": 5}),
			}, nil
		},
	})

	if err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 1_000_000); err != nil {
		t.Fatalf("expected no limit for an unset key, got %v", err)
	}
}

func TestEnforceLimit_DatabaseErrorPropagates(t *testing.T) {
	dbErr := errors.New("connection reset")
	svc := NewService(&mockSubStore{
		getOrgSubscriptionWithPlan: func(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error) {
			return db.GetOrgSubscriptionWithPlanRow{}, dbErr
		},
	})

	err := svc.EnforceLimit(context.Background(), uuid.New(), "max_members", 0)
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected the raw db error to propagate, got %v", err)
	}
}

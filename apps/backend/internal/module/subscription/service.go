// Package subscription resolves plan-limit values for an organization,
// mirroring SubscriptionService in the source app
// (src/modules/subscription/service.ts). Only the limit-resolution surface
// needed by the organization module's invite flow (Phase 3) is implemented
// here; the /subscription HTTP endpoints (get, assign) land in Phase 4.
package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"maps"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/controlplane/backend/internal/infra/database"
	"github.com/controlplane/backend/internal/infra/database/db"
	"github.com/controlplane/backend/internal/shared/apperror"
)

var _ subStore = (*database.Store)(nil)

// subStore is the subset of *database.Store this service depends on.
type subStore interface {
	GetOrgSubscriptionWithPlan(ctx context.Context, organizationID uuid.UUID) (db.GetOrgSubscriptionWithPlanRow, error)
}

// Service resolves and enforces subscription plan limits.
type Service struct {
	store subStore
}

// NewService builds a subscription Service.
func NewService(store subStore) *Service {
	return &Service{store: store}
}

// GetLimit resolves the numeric limit for key, plan limits overlaid by
// custom_limits (custom wins on conflict). Returns nil when the org has no
// subscription at all — the caller treats that as unlimited, mirroring the
// source's `if (!sub) return null`.
func (s *Service) GetLimit(ctx context.Context, organizationID uuid.UUID, key string) (*float64, error) {
	row, err := s.store.GetOrgSubscriptionWithPlan(ctx, organizationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	limits := map[string]float64{}
	if len(row.PlanLimits) > 0 {
		_ = json.Unmarshal(row.PlanLimits, &limits)
	}
	if len(row.CustomLimits) > 0 {
		custom := map[string]float64{}
		if json.Unmarshal(row.CustomLimits, &custom) == nil {
			maps.Copy(limits, custom)
		}
	}

	if v, ok := limits[key]; ok {
		return &v, nil
	}
	return nil, nil
}

// EnforceLimit returns apperror.LimitExceeded when currentCount has reached
// or passed the resolved limit. No subscription, no limit for key, or a
// limit of -1 (enterprise/unlimited) all pass. Mirrors
// SubscriptionService.enforceLimit exactly.
func (s *Service) EnforceLimit(ctx context.Context, organizationID uuid.UUID, key string, currentCount int) error {
	limit, err := s.GetLimit(ctx, organizationID, key)
	if err != nil {
		return err
	}
	if limit == nil || *limit == -1 {
		return nil
	}
	if float64(currentCount) >= *limit {
		return apperror.New(apperror.LimitExceeded)
	}
	return nil
}

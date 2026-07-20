// Package auditlog records best-effort audit trail entries, mirroring
// AuditLogService in the source app (src/modules/audit-log/service.ts).
// Query endpoints land in Phase 4 — this phase only writes.
package auditlog

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/controlplane/backend/internal/infra/database/db"
)

// Recorded audit actions. Mirrors AuditAction in the source app; role
// actions are added in Phase 4.
const (
	ActionUserLogin        = "user.login"
	ActionUserRegister     = "user.register"
	ActionOrgCreated       = "org.created"
	ActionOrgMemberInvited = "org.member.invited"
)

// Service records audit log entries. Writes are best-effort: a failure is
// logged and swallowed, never propagated to the caller, per CLAUDE.md
// ("Audit-log writes are best-effort: log failures, never fail the
// request").
type Service struct {
	q   db.Querier
	log *slog.Logger
}

// NewService builds an auditlog Service. q is typically a *database.Store
// (outside a transaction) or a tx-bound *db.Queries (inside one) — both
// satisfy db.Querier.
func NewService(q db.Querier, log *slog.Logger) *Service {
	return &Service{q: q, log: log}
}

// Record inserts one audit row for action. userID and organizationID are
// optional (nil omits the column); metadata is an optional raw JSON blob.
// Errors are logged, not returned — this must never fail the caller's
// request.
func (s *Service) Record(ctx context.Context, action string, userID, organizationID *uuid.UUID, metadata []byte) {
	err := s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: toPgUUID(organizationID),
		UserID:         toPgUUID(userID),
		Action:         action,
		Metadata:       metadata,
	})
	if err != nil {
		s.log.Error("failed to record audit log", "error", err, "action", action)
	}
}

func toPgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

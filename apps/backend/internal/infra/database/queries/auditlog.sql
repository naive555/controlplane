-- name: CreateAuditLog :exec
INSERT INTO audit_logs (organization_id, user_id, action, metadata)
VALUES ($1, $2, $3, $4);

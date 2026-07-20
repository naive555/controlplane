-- name: GetOrgSubscriptionWithPlan :one
SELECT s.custom_limits, p.limits AS plan_limits
FROM org_subscriptions s
JOIN plans p ON p.id = s.plan_id
WHERE s.organization_id = $1;

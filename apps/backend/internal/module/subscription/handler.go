package subscription

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/controlplane/backend/internal/infra/database/db"
	appmw "github.com/controlplane/backend/internal/middleware"
	"github.com/controlplane/backend/internal/shared/httpx"
)

// Handler implements the two /subscription routes, mirroring
// src/modules/subscription/index.ts.
type Handler struct {
	service *Service
}

// NewHandler builds a subscription Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the two /subscription routes on the given group. Both are
// org-scoped per docs/02-api-contract.md.
func (h *Handler) Register(g *echo.Group, guards *appmw.Guards) {
	g.GET("", h.get, guards.RequireOrg())
	g.POST("/assign", h.assign, guards.RequireOrg())
}

func (h *Handler) get(c echo.Context) error {
	sub, err := h.service.GetSubscription(c.Request().Context(), appmw.OrgID(c))
	if err != nil {
		return err
	}
	if sub == nil {
		return c.JSON(http.StatusOK, nil)
	}

	return c.JSON(http.StatusOK, toSubscriptionResponse(*sub))
}

func (h *Handler) assign(c echo.Context) error {
	var req AssignRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}

	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		return err
	}

	if err := h.service.AssignPlan(c.Request().Context(), appmw.OrgID(c), planID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, SuccessResponse{Success: true})
}

func toSubscriptionResponse(row db.GetOrgSubscriptionRow) SubscriptionResponse {
	var customLimits json.RawMessage
	if len(row.CustomLimits) > 0 {
		customLimits = row.CustomLimits
	}

	return SubscriptionResponse{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		PlanID:         row.PlanID,
		CustomLimits:   customLimits,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		Plan: PlanResponse{
			ID:        row.PlanPid,
			Name:      row.PlanName,
			Limits:    row.PlanPlimits,
			CreatedAt: row.PlanCreatedAt,
		},
	}
}

package organization

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/controlplane/backend/internal/infra/database/db"
	appmw "github.com/controlplane/backend/internal/middleware"
	"github.com/controlplane/backend/internal/shared/apperror"
	"github.com/controlplane/backend/internal/shared/httpx"
)

// Handler implements the four /organizations routes, mirroring
// src/modules/organization/index.ts.
type Handler struct {
	service *Service
}

// NewHandler builds an organization Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the four /organizations routes on the given group, guarded
// per docs/02-api-contract.md: create/list are auth-only, invite/remove are
// org-scoped.
func (h *Handler) Register(g *echo.Group, guards *appmw.Guards) {
	g.POST("", h.create, guards.RequireAuth())
	g.GET("", h.list, guards.RequireAuth())
	g.POST("/invite", h.invite, guards.RequireOrg())
	g.DELETE("/members/:userId", h.removeMember, guards.RequireOrg())
}

func (h *Handler) create(c echo.Context) error {
	var req CreateRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}

	org, err := h.service.Create(c.Request().Context(), appmw.UserID(c), req.Name, req.Slug)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, toOrgResponse(org))
}

func (h *Handler) list(c echo.Context) error {
	rows, err := h.service.ListByUser(c.Request().Context(), appmw.UserID(c))
	if err != nil {
		return err
	}

	out := make([]MembershipResponse, len(rows))
	for i, row := range rows {
		out[i] = toMembershipResponse(row)
	}

	return c.JSON(http.StatusOK, out)
}

func (h *Handler) invite(c echo.Context) error {
	var req InviteRequest
	if err := httpx.BindAndValidate(c, &req); err != nil {
		return err
	}

	membership := appmw.MembershipFromContext(c)
	err := h.service.Invite(c.Request().Context(), appmw.OrgID(c), appmw.UserID(c), membership.Role, req.Email, req.Role)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, SuccessResponse{Success: true})
}

func (h *Handler) removeMember(c echo.Context) error {
	targetID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		// A malformed id can never match a member row.
		return apperror.New(apperror.MemberNotFound)
	}

	membership := appmw.MembershipFromContext(c)
	if err := h.service.RemoveMember(c.Request().Context(), appmw.OrgID(c), membership.Role, targetID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, SuccessResponse{Success: true})
}

func toOrgResponse(org db.Organization) OrgResponse {
	return OrgResponse{
		ID:        org.ID,
		Name:      org.Name,
		Slug:      org.Slug,
		CreatedAt: org.CreatedAt,
		UpdatedAt: org.UpdatedAt,
	}
}

func toMembershipResponse(row db.ListMembershipsByUserRow) MembershipResponse {
	return MembershipResponse{
		ID:             row.ID,
		UserID:         row.UserID,
		OrganizationID: row.OrganizationID,
		Role:           row.Role,
		CreatedAt:      row.CreatedAt,
		Organization: OrgResponse{
			ID:        row.OrgID,
			Name:      row.OrgName,
			Slug:      row.OrgSlug,
			CreatedAt: row.OrgCreatedAt,
			UpdatedAt: row.OrgUpdatedAt,
		},
	}
}

// Package server wires the Echo instance: middleware, error handling, and
// route registration. Module handlers plug into New in later phases.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/controlplane/backend/internal/config"
	"github.com/controlplane/backend/internal/infra/database"
	appredis "github.com/controlplane/backend/internal/infra/redis"
	appmw "github.com/controlplane/backend/internal/middleware"
	"github.com/controlplane/backend/internal/module/auditlog"
	"github.com/controlplane/backend/internal/module/auth"
	"github.com/controlplane/backend/internal/module/health"
	"github.com/controlplane/backend/internal/module/organization"
	"github.com/controlplane/backend/internal/module/rbac"
	"github.com/controlplane/backend/internal/module/subscription"
	"github.com/controlplane/backend/internal/shared/apperror"
)

// New builds a fully configured Echo instance: middleware stack, custom
// error handler, infra-backed module wiring, and route registration.
// RequirePermission (RBAC) lands in Phase 4.
func New(cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool, rdb *redis.Client) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.HTTPErrorHandler = newErrorHandler(log)
	e.Validator = newRequestValidator()

	e.Use(echomw.Recover())
	e.Use(appmw.RequestID())
	e.Use(requestLogger(log))

	health.NewHandler().Register(e)

	store := database.NewStore(pool)
	redisAuth := appredis.NewAuth(rdb)
	tokenSvc := auth.NewTokenService(cfg)
	auditSvc := auditlog.NewService(store, log)

	authSvc := auth.NewService(store, redisAuth, auditSvc)
	authHandler := auth.NewHandler(authSvc, tokenSvc, store, redisAuth, cfg.JWTRefreshExpiresIn)
	authHandler.Register(e.Group("/auth"))

	rbacSvc := rbac.NewService(store)
	guards := appmw.NewGuards(tokenSvc, redisAuth, store, rbacSvc)
	subSvc := subscription.NewService(store)
	orgSvc := organization.NewService(store, auditSvc, subSvc)
	orgHandler := organization.NewHandler(orgSvc)
	orgHandler.Register(e.Group("/organizations"), guards)

	return e
}

// errorBody is the JSON shape returned for every error response,
// mirroring the { message: string } shape used by the source app.
type errorBody struct {
	Message string `json:"message"`
}

// newErrorHandler returns Echo's global error handler. It maps:
//   - *apperror.Error   -> apperror.Resolve(code)
//   - *echo.HTTPError    -> its status/message (404 normalized to "Route not found";
//     handlers use httpx.BindAndValidate to produce 400 "Invalid request body" /
//     422 "Validation failed" as *echo.HTTPError)
//   - anything else      -> logged at error level, 500 "Internal server error"
func newErrorHandler(log *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		status := http.StatusInternalServerError
		message := "Internal server error"

		var appErr *apperror.Error
		var httpErr *echo.HTTPError

		switch {
		case asAppError(err, &appErr):
			status, message = apperror.Resolve(appErr.Code)

		case asHTTPError(err, &httpErr):
			status = httpErr.Code
			if status == http.StatusNotFound {
				message = "Route not found"
			} else if msg, ok := httpErr.Message.(string); ok {
				message = msg
			}

		default:
			log.Error("unhandled error", "error", err, "path", c.Request().URL.Path)
		}

		if status >= 500 {
			log.Error("request failed", "error", err, "status", status, "path", c.Request().URL.Path)
		}

		if writeErr := c.JSON(status, errorBody{Message: message}); writeErr != nil {
			log.Error("failed to write error response", "error", writeErr)
		}
	}
}

func asAppError(err error, target **apperror.Error) bool {
	if e, ok := err.(*apperror.Error); ok {
		*target = e
		return true
	}
	return false
}

func asHTTPError(err error, target **echo.HTTPError) bool {
	if e, ok := err.(*echo.HTTPError); ok {
		*target = e
		return true
	}
	return false
}

// requestLogger logs each request at info level via slog.
func requestLogger(log *slog.Logger) echo.MiddlewareFunc {
	return echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, v echomw.RequestLoggerValues) error {
			log.Info("request",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency", v.Latency.String(),
				"request_id", v.RequestID,
			)
			return nil
		},
	})
}

// Shutdown gracefully stops the server and closes infrastructure clients.
func Shutdown(ctx context.Context, e *echo.Echo, pool *pgxpool.Pool, rdb *redis.Client) error {
	if err := e.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown echo: %w", err)
	}
	pool.Close()
	if err := rdb.Close(); err != nil {
		return fmt.Errorf("close redis: %w", err)
	}
	return nil
}

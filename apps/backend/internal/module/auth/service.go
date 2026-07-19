package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/controlplane/backend/internal/infra/database"
	"github.com/controlplane/backend/internal/infra/database/db"
	"github.com/controlplane/backend/internal/infra/redis"
	"github.com/controlplane/backend/internal/module/auditlog"
	"github.com/controlplane/backend/internal/shared/apperror"
)

// Compile-time checks that the concrete infra types satisfy the narrow
// interfaces this service depends on.
var (
	_ authStore    = (*database.Store)(nil)
	_ loginLimiter = (*redis.Auth)(nil)
)

// maxLoginAttempts mirrors MAX_LOGIN_ATTEMPTS in the source app's
// src/modules/auth/service.ts.
const maxLoginAttempts = 5

// bcryptCost mirrors SALT_ROUNDS in the source app's
// src/modules/auth/service.ts (bcryptjs cost 12).
const bcryptCost = 12

// authStore is the subset of *database.Store the auth service depends on,
// narrowed so unit tests can hand-mock it without implementing the full
// db.Querier surface. *database.Store satisfies this (it embeds *db.Queries
// and provides WithTx).
type authStore interface {
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
	CreateUser(ctx context.Context, arg db.CreateUserParams) (db.User, error)
	GetSessionByRefreshToken(ctx context.Context, refreshToken string) (db.Session, error)
	CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error)
	RevokeSessionByID(ctx context.Context, id uuid.UUID) error
	RevokeSessionFamily(ctx context.Context, family uuid.UUID) error
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error
	WithTx(ctx context.Context, fn func(q *db.Queries) error) error
}

// loginLimiter is the subset of *redis.Auth the service needs for login
// rate limiting, narrowed for the same reason as authStore.
type loginLimiter interface {
	GetLoginAttempts(ctx context.Context, email string) (int, error)
	IncrementLoginAttempts(ctx context.Context, email string) (int64, error)
	ResetLoginAttempts(ctx context.Context, email string) error
}

// Service implements register/login/session-rotation, mirroring AuthService
// in the source app's src/modules/auth/service.ts.
type Service struct {
	store   authStore
	limiter loginLimiter
	audit   *auditlog.Service
}

// NewService builds an auth Service.
func NewService(store authStore, limiter loginLimiter, audit *auditlog.Service) *Service {
	return &Service{store: store, limiter: limiter, audit: audit}
}

// Register creates a new user. passwordHash is the already-bcrypt-hashed
// password (hashing happens in the handler alongside the 72-byte
// truncation shared with Login). Returns apperror.EmailTaken if the email
// is already registered.
func (s *Service) Register(ctx context.Context, email, passwordHash string, displayName *string) (db.User, error) {
	_, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		return db.User{}, apperror.New(apperror.EmailTaken)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, err
	}

	user, err := s.store.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
	})
	if err != nil {
		return db.User{}, err
	}

	s.audit.Record(ctx, auditlog.ActionUserRegister, &user.ID, nil, nil)

	return user, nil
}

// Login validates credentials against the rate limiter and stored hash.
// The rate-limit check happens BEFORE credential validation, matching
// source. A failed attempt (unknown email or bad password) increments the
// limiter and returns apperror.InvalidCredentials; success resets it.
func (s *Service) Login(ctx context.Context, email, password string) (db.User, error) {
	attempts, err := s.limiter.GetLoginAttempts(ctx, email)
	if err != nil {
		return db.User{}, err
	}
	if attempts >= maxLoginAttempts {
		return db.User{}, apperror.New(apperror.TooManyAttempts)
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, err
		}
		if _, incErr := s.limiter.IncrementLoginAttempts(ctx, email); incErr != nil {
			return db.User{}, incErr
		}
		return db.User{}, apperror.New(apperror.InvalidCredentials)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), truncatePassword(password)); err != nil {
		if _, incErr := s.limiter.IncrementLoginAttempts(ctx, email); incErr != nil {
			return db.User{}, incErr
		}
		return db.User{}, apperror.New(apperror.InvalidCredentials)
	}

	if err := s.limiter.ResetLoginAttempts(ctx, email); err != nil {
		return db.User{}, err
	}

	s.audit.Record(ctx, auditlog.ActionUserLogin, &user.ID, nil, nil)

	return user, nil
}

// RotateSession validates oldRefreshToken, detects reuse of an already-
// rotated (revoked) token by revoking its entire family, and — if valid —
// atomically revokes it and inserts newRefreshToken in its place. Mirrors
// AuthService.rotateSession exactly, including check order: not-found,
// then reuse, then expiry.
func (s *Service) RotateSession(ctx context.Context, oldRefreshToken, newRefreshToken string, expiresAt time.Time) (uuid.UUID, error) {
	var userID uuid.UUID

	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		session, err := q.GetSessionByRefreshToken(ctx, oldRefreshToken)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperror.New(apperror.InvalidRefreshToken)
			}
			return err
		}

		if session.IsRevoked {
			if err := q.RevokeSessionFamily(ctx, session.Family); err != nil {
				return err
			}
			return apperror.New(apperror.RefreshTokenReuse)
		}

		if session.ExpiresAt.Before(time.Now()) {
			return apperror.New(apperror.RefreshTokenExpired)
		}

		if err := q.RevokeSessionByID(ctx, session.ID); err != nil {
			return err
		}

		if _, err := q.CreateSession(ctx, db.CreateSessionParams{
			UserID:       session.UserID,
			RefreshToken: newRefreshToken,
			Family:       session.Family,
			ExpiresAt:    expiresAt,
		}); err != nil {
			return err
		}

		userID = session.UserID
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}

	return userID, nil
}

// CreateSession inserts a new session row, used by register/login to
// establish the initial refresh-token family.
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, refreshToken string, family uuid.UUID, expiresAt time.Time) error {
	_, err := s.store.CreateSession(ctx, db.CreateSessionParams{
		UserID:       userID,
		RefreshToken: refreshToken,
		Family:       family,
		ExpiresAt:    expiresAt,
	})
	return err
}

// RevokeAllSessions revokes every active session for a user, used by
// logout.
func (s *Service) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	return s.store.RevokeAllUserSessions(ctx, userID)
}

// truncatePassword reproduces bcryptjs's silent truncation of inputs over
// 72 bytes (Go's bcrypt errors instead of truncating), so long passwords
// hash/compare identically to the source rather than turning into a 500.
func truncatePassword(password string) []byte {
	b := []byte(password)
	if len(b) > 72 {
		return b[:72]
	}
	return b
}

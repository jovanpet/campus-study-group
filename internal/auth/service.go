package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/jovanpet/campus-study-group/internal/user"
)

const maxDisplayNameRunes = 50

// Apple is the part of Sign in with Apple the service needs; *AppleClient
// implements it and tests fake it.
type Apple interface {
	VerifyIdentityToken(ctx context.Context, token, rawNonce string) (Identity, error)
	ExchangeCode(ctx context.Context, code string) (refreshToken string, err error)
	Revoke(ctx context.Context, refreshToken string) error
}

// Users is the part of user.Repository the service needs.
type Users interface {
	UpsertByAppleSub(ctx context.Context, appleSub string, displayName *string, refreshTokenEnc []byte) (user.User, error)
	AppleRefreshToken(ctx context.Context, id uuid.UUID) ([]byte, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type Service struct {
	apple  Apple // nil when Apple credentials are not configured (development)
	users  Users
	tokens TokenRepository
	sealer *sealer
	now    func() time.Time
}

// NewService builds the auth service. apple may be nil, in which case Apple
// sign-in returns ErrNotConfigured. secret keys the encryption of stored
// Apple refresh tokens.
func NewService(apple Apple, users Users, tokens TokenRepository, secret string) (*Service, error) {
	s, err := newSealer(secret)
	if err != nil {
		return nil, err
	}
	return &Service{apple: apple, users: users, tokens: tokens, sealer: s, now: time.Now}, nil
}

type SignInInput struct {
	IdentityToken     string
	AuthorizationCode string
	// Nonce is the raw nonce whose SHA-256 the app passed to Apple; optional.
	Nonce string
	// DisplayName is the name Apple gives the app on first authorization only.
	DisplayName string
}

// SignInWithApple verifies the Apple credentials, finds or creates the user
// and returns a new API token. A new user is not onboarded yet
// (user.IsOnboarded) until their profile is completed.
func (s *Service) SignInWithApple(ctx context.Context, in SignInInput) (string, user.User, error) {
	if in.IdentityToken == "" || in.AuthorizationCode == "" {
		return "", user.User{}, fmt.Errorf("%w: identity_token and authorization_code are required", ErrInvalidRequest)
	}
	if s.apple == nil {
		return "", user.User{}, ErrNotConfigured
	}
	identity, err := s.apple.VerifyIdentityToken(ctx, in.IdentityToken, in.Nonce)
	if err != nil {
		return "", user.User{}, err
	}
	refreshToken, err := s.apple.ExchangeCode(ctx, in.AuthorizationCode)
	if err != nil {
		return "", user.User{}, err
	}
	return s.signIn(ctx, identity.Sub, in.DisplayName, s.sealer.seal(refreshToken))
}

// SignInDev signs in without Apple, for local development and manual testing.
// Callers must only expose it outside production. The "dev:" prefix keeps
// these accounts apart from real Apple subjects.
func (s *Service) SignInDev(ctx context.Context, name, displayName string) (string, user.User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	return s.signIn(ctx, "dev:"+name, displayName, nil)
}

func (s *Service) signIn(ctx context.Context, appleSub, displayName string, refreshTokenEnc []byte) (string, user.User, error) {
	var name *string
	if dn := strings.TrimSpace(displayName); dn != "" {
		if r := []rune(dn); len(r) > maxDisplayNameRunes {
			dn = string(r[:maxDisplayNameRunes])
		}
		name = &dn
	}
	u, err := s.users.UpsertByAppleSub(ctx, appleSub, name, refreshTokenEnc)
	if err != nil {
		return "", user.User{}, err
	}
	token, hash := newToken()
	if err := s.tokens.Create(ctx, u.ID, hash, s.now().Add(tokenTTL)); err != nil {
		return "", user.User{}, err
	}
	return token, u, nil
}

// Authenticate returns the user a token belongs to, or ErrInvalidToken. Using
// a token pushes its expiry out, at most once per tokenRefreshAfter.
func (s *Service) Authenticate(ctx context.Context, token string) (uuid.UUID, error) {
	if token == "" {
		return uuid.UUID{}, ErrInvalidToken
	}
	hash, now := hashToken(token), s.now()
	userID, expiresAt, err := s.tokens.Lookup(ctx, hash, now)
	if err != nil {
		return uuid.UUID{}, err
	}
	if expiresAt.Sub(now) < tokenTTL-tokenRefreshAfter {
		if err := s.tokens.Extend(ctx, hash, now.Add(tokenTTL)); err != nil {
			// The token is still valid; not worth failing the request.
			slog.WarnContext(ctx, "extend token", "error", err)
		}
	}
	return userID, nil
}

// Logout invalidates one token; the user's other devices stay signed in.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.tokens.Delete(ctx, hashToken(token))
}

// DeleteAccount revokes the user's Apple grant and deletes the user and
// everything that references them. If Apple can't be reached the account is
// kept and the error returned, so the client can retry.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	enc, err := s.users.AppleRefreshToken(ctx, userID)
	if errors.Is(err, user.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(enc) > 0 {
		refreshToken, err := s.sealer.open(enc)
		switch {
		case err != nil:
			// Unreadable (e.g. SESSION_SECRET rotated): nothing to revoke with,
			// and refusing would make the account undeletable.
			slog.WarnContext(ctx, "apple refresh token unreadable; deleting without revoke", "user_id", userID.String(), "error", err)
		case s.apple == nil:
			slog.WarnContext(ctx, "apple not configured; deleting without revoke", "user_id", userID.String())
		default:
			if err := s.apple.Revoke(ctx, refreshToken); err != nil {
				return err
			}
		}
	}
	return s.users.Delete(ctx, userID)
}

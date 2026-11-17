// Package auth handles Sign in with Apple and API tokens.
//
// Sign-in: the app sends Apple's identity token and authorization code. The
// identity token proves who the user is; the code is exchanged for an Apple
// refresh token, kept (encrypted) only so the Apple grant can be revoked when
// the account is deleted. The app gets back an opaque API token to send as
// "Authorization: Bearer <token>"; only its hash is stored.
package auth

import "errors"

var (
	// ErrInvalidRequest: the sign-in request is missing fields.
	ErrInvalidRequest = errors.New("invalid request")
	// ErrInvalidAppleToken: the identity token failed verification.
	ErrInvalidAppleToken = errors.New("invalid apple identity token")
	// ErrInvalidGrant: Apple rejected the authorization code (used or expired).
	ErrInvalidGrant = errors.New("invalid apple authorization code")
	// ErrAppleUnavailable: Apple could not be reached or returned an unexpected error.
	ErrAppleUnavailable = errors.New("apple sign-in unavailable")
	// ErrNotConfigured: the Apple credentials are not set (development only).
	ErrNotConfigured = errors.New("apple sign-in is not configured")
	// ErrInvalidToken: the API token is unknown or expired.
	ErrInvalidToken = errors.New("invalid or expired token")
)

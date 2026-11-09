// Package auth handles Sign in with Apple and API session tokens.
//
// TODO:
//   - verify Apple identity tokens (config.Apple: team ID, key ID, private key)
//   - find-or-create the user via user.Service
//   - issue/verify session tokens signed with SESSION_SECRET
//   - middleware that puts the authenticated user ID on the request context
package auth

type Service struct{}

func NewService() *Service {
	return &Service{}
}

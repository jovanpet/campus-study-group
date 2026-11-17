package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jovanpet/campus-study-group/internal/httpapi"
	"github.com/jovanpet/campus-study-group/internal/user"
)

type stubApple struct {
	verifyErr, exchangeErr, revokeErr error
	gotNonce                          string
	revoked                           []string
}

func (a *stubApple) VerifyIdentityToken(_ context.Context, token, nonce string) (Identity, error) {
	a.gotNonce = nonce
	return Identity{Sub: "sub-" + token}, a.verifyErr
}

func (a *stubApple) ExchangeCode(_ context.Context, code string) (string, error) {
	return "refresh-" + code, a.exchangeErr
}

func (a *stubApple) Revoke(_ context.Context, refreshToken string) error {
	if a.revokeErr != nil {
		return a.revokeErr
	}
	a.revoked = append(a.revoked, refreshToken)
	return nil
}

type memUsers struct {
	bySub   map[string]*user.User
	refresh map[uuid.UUID][]byte
}

func newMemUsers() *memUsers {
	return &memUsers{bySub: map[string]*user.User{}, refresh: map[uuid.UUID][]byte{}}
}

func (m *memUsers) UpsertByAppleSub(_ context.Context, sub string, displayName *string, enc []byte) (user.User, error) {
	u, ok := m.bySub[sub]
	if !ok {
		u = &user.User{ID: uuid.New(), AppleSub: sub}
		m.bySub[sub] = u
	}
	if u.DisplayName == nil {
		u.DisplayName = displayName
	}
	if enc != nil {
		m.refresh[u.ID] = enc
	}
	return *u, nil
}

func (m *memUsers) AppleRefreshToken(_ context.Context, id uuid.UUID) ([]byte, error) {
	for _, u := range m.bySub {
		if u.ID == id {
			return m.refresh[id], nil
		}
	}
	return nil, user.ErrNotFound
}

func (m *memUsers) Delete(_ context.Context, id uuid.UUID) error {
	for sub, u := range m.bySub {
		if u.ID == id {
			delete(m.bySub, sub)
		}
	}
	return nil
}

type memToken struct {
	userID    uuid.UUID
	expiresAt time.Time
}

type memTokens map[string]*memToken

func (m memTokens) Create(_ context.Context, userID uuid.UUID, hash []byte, expiresAt time.Time) error {
	m[string(hash)] = &memToken{userID, expiresAt}
	return nil
}

func (m memTokens) Lookup(_ context.Context, hash []byte, now time.Time) (uuid.UUID, time.Time, error) {
	t, ok := m[string(hash)]
	if !ok || !t.expiresAt.After(now) {
		return uuid.UUID{}, time.Time{}, ErrInvalidToken
	}
	return t.userID, t.expiresAt, nil
}

func (m memTokens) Extend(_ context.Context, hash []byte, expiresAt time.Time) error {
	m[string(hash)].expiresAt = expiresAt
	return nil
}

func (m memTokens) Delete(_ context.Context, hash []byte) error {
	delete(m, string(hash))
	return nil
}

type fixture struct {
	svc    *Service
	apple  *stubApple
	users  *memUsers
	tokens memTokens
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{apple: &stubApple{}, users: newMemUsers(), tokens: memTokens{}, now: time.Now()}
	svc, err := NewService(f.apple, f.users, f.tokens, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return f.now }
	f.svc = svc
	return f
}

var signIn = SignInInput{IdentityToken: "alice", AuthorizationCode: "code", Nonce: "n", DisplayName: "  Alice A  "}

func TestSignInCreatesNotOnboardedUserOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	token, u, err := f.svc.SignInWithApple(ctx, signIn)
	if err != nil {
		t.Fatalf("SignInWithApple: %v", err)
	}
	if u.IsOnboarded() {
		t.Error("new user should not be onboarded")
	}
	if u.DisplayName == nil || *u.DisplayName != "Alice A" {
		t.Errorf("DisplayName = %v, want trimmed \"Alice A\"", u.DisplayName)
	}
	if f.apple.gotNonce != "n" {
		t.Errorf("nonce passed to Apple = %q", f.apple.gotNonce)
	}

	// The stored Apple refresh token is encrypted, and decrypts to the original.
	enc := f.users.refresh[u.ID]
	if bytes.Contains(enc, []byte("refresh-code")) {
		t.Error("refresh token stored in plaintext")
	}
	if plain, err := f.svc.sealer.open(enc); err != nil || plain != "refresh-code" {
		t.Errorf("open = %q, %v", plain, err)
	}
	// Only the token's hash is stored.
	if _, ok := f.tokens[token]; ok {
		t.Error("API token stored in plaintext")
	}

	got, err := f.svc.Authenticate(ctx, token)
	if err != nil || got != u.ID {
		t.Errorf("Authenticate = %v, %v; want %v", got, err, u.ID)
	}

	// Signing in again (app relaunched before onboarding, or a second device)
	// returns the same user with a different token; both tokens work.
	again := signIn
	again.DisplayName = ""
	token2, u2, err := f.svc.SignInWithApple(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u.ID || token2 == token || len(f.users.bySub) != 1 {
		t.Errorf("second sign-in: user %v (want %v), users=%d", u2.ID, u.ID, len(f.users.bySub))
	}
	if _, err := f.svc.Authenticate(ctx, token); err != nil {
		t.Errorf("first token after second sign-in: %v", err)
	}
}

func TestSignInErrors(t *testing.T) {
	ctx := context.Background()

	f := newFixture(t)
	if _, _, err := f.svc.SignInWithApple(ctx, SignInInput{IdentityToken: "x"}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("missing code: %v", err)
	}

	f.apple.verifyErr = ErrInvalidAppleToken
	if _, _, err := f.svc.SignInWithApple(ctx, signIn); !errors.Is(err, ErrInvalidAppleToken) {
		t.Errorf("bad identity token: %v", err)
	}
	f.apple.verifyErr, f.apple.exchangeErr = nil, ErrInvalidGrant
	if _, _, err := f.svc.SignInWithApple(ctx, signIn); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("bad code: %v", err)
	}
	if len(f.users.bySub) != 0 || len(f.tokens) != 0 {
		t.Error("failed sign-ins must not create users or tokens")
	}

	unconfigured, err := NewService(nil, newMemUsers(), memTokens{}, "s")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := unconfigured.SignInWithApple(ctx, signIn); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no Apple config: %v", err)
	}
}

func TestTokenLifetime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	token, _, err := f.svc.SignInWithApple(ctx, signIn)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "unknown"} {
		if _, err := f.svc.Authenticate(ctx, bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("Authenticate(%q) = %v", bad, err)
		}
	}

	// Use within the TTL slides the expiry forward...
	f.now = f.now.Add(tokenTTL - time.Hour)
	if _, err := f.svc.Authenticate(ctx, token); err != nil {
		t.Fatalf("near expiry: %v", err)
	}
	f.now = f.now.Add(tokenTTL - time.Hour)
	if _, err := f.svc.Authenticate(ctx, token); err != nil {
		t.Fatalf("after sliding: %v", err)
	}
	// ...but an unused token expires.
	f.now = f.now.Add(tokenTTL + time.Second)
	if _, err := f.svc.Authenticate(ctx, token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expired: %v", err)
	}
}

func TestLogoutInvalidatesOnlyThatToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	phone, _, _ := f.svc.SignInWithApple(ctx, signIn)
	tablet, _, _ := f.svc.SignInWithApple(ctx, signIn)

	if err := f.svc.Logout(ctx, phone); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(ctx, phone); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("logged-out token: %v", err)
	}
	if _, err := f.svc.Authenticate(ctx, tablet); err != nil {
		t.Errorf("other device: %v", err)
	}
}

func TestDeleteAccount(t *testing.T) {
	ctx := context.Background()

	f := newFixture(t)
	_, u, _ := f.svc.SignInWithApple(ctx, signIn)
	if err := f.svc.DeleteAccount(ctx, u.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if len(f.apple.revoked) != 1 || f.apple.revoked[0] != "refresh-code" {
		t.Errorf("revoked = %v, want the decrypted refresh token", f.apple.revoked)
	}
	if len(f.users.bySub) != 0 {
		t.Error("user not deleted")
	}
	if err := f.svc.DeleteAccount(ctx, u.ID); err != nil {
		t.Errorf("deleting twice: %v", err)
	}

	// Apple unreachable: keep the account so the client can retry.
	f = newFixture(t)
	_, u, _ = f.svc.SignInWithApple(ctx, signIn)
	f.apple.revokeErr = ErrAppleUnavailable
	if err := f.svc.DeleteAccount(ctx, u.ID); !errors.Is(err, ErrAppleUnavailable) {
		t.Errorf("err = %v, want ErrAppleUnavailable", err)
	}
	if len(f.users.bySub) != 1 {
		t.Error("account deleted even though the Apple revoke failed")
	}

	// Unreadable refresh token (secret rotated): still deletable.
	f = newFixture(t)
	_, u, _ = f.svc.SignInWithApple(ctx, signIn)
	f.users.refresh[u.ID] = []byte("corrupted-ciphertext-corrupted-ciphertext")
	if err := f.svc.DeleteAccount(ctx, u.ID); err != nil || len(f.users.bySub) != 0 {
		t.Errorf("unreadable token: err = %v, users = %d", err, len(f.users.bySub))
	}

	// Dev accounts have no Apple token: nothing to revoke.
	f = newFixture(t)
	_, u, _ = f.svc.SignInDev(ctx, "bob", "")
	if err := f.svc.DeleteAccount(ctx, u.ID); err != nil || len(f.apple.revoked) != 0 || len(f.users.bySub) != 0 {
		t.Errorf("dev account: err = %v, revoked = %v", err, f.apple.revoked)
	}
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandler(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	NewHandler(f.svc, false).RegisterRoutes(mux)
	// A protected route from another package, to exercise the middleware.
	mux.Handle("GET /whoami", RequireUser(f.svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := httpapi.UserID(r.Context())
		httpapi.WriteJSON(w, http.StatusOK, id)
	})))

	rec := do(mux, "POST", "/auth/apple", "", `{"identity_token":"alice","authorization_code":"code"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign in = %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"onboarded":false`) {
		t.Errorf("sign-in response missing onboarded flag: %s", rec.Body)
	}
	token, _, _ := f.svc.SignInWithApple(context.Background(), signIn)

	cases := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{"no token", "GET", "/whoami", "", "", http.StatusUnauthorized},
		{"bad token", "GET", "/whoami", "nope", "", http.StatusUnauthorized},
		{"good token", "GET", "/whoami", token, "", http.StatusOK},
		{"missing fields", "POST", "/auth/apple", "", `{}`, http.StatusBadRequest},
		{"unknown field", "POST", "/auth/apple", "", `{"x":1}`, http.StatusBadRequest},
		{"dev login off", "POST", "/auth/dev", "", `{}`, http.StatusNotFound},
		{"logout without token", "POST", "/auth/logout", "", "", http.StatusUnauthorized},
		{"logout", "POST", "/auth/logout", token, "", http.StatusNoContent},
		{"token after logout", "GET", "/whoami", token, "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		if rec := do(mux, tc.method, tc.path, tc.token, tc.body); rec.Code != tc.want {
			t.Errorf("%s: %s %s = %d, want %d (%s)", tc.name, tc.method, tc.path, rec.Code, tc.want, rec.Body)
		}
	}

	f.apple.verifyErr = ErrInvalidAppleToken
	if rec := do(mux, "POST", "/auth/apple", "", `{"identity_token":"a","authorization_code":"c"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("rejected identity token = %d, want 401", rec.Code)
	}
	f.apple.verifyErr = ErrAppleUnavailable
	if rec := do(mux, "POST", "/auth/apple", "", `{"identity_token":"a","authorization_code":"c"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("apple down = %d, want 502", rec.Code)
	}
	f.apple.verifyErr = nil

	token, _, _ = f.svc.SignInWithApple(context.Background(), signIn)
	if rec := do(mux, "DELETE", "/me", token, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE /me = %d, want 204", rec.Code)
	}
}

func TestDevLogin(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	NewHandler(f.svc, true).RegisterRoutes(mux)

	rec := do(mux, "POST", "/auth/dev", "", `{"name":"bob","display_name":"Bob"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("POST /auth/dev = %d %s", rec.Code, rec.Body)
	}
	if _, ok := f.users.bySub["dev:bob"]; !ok {
		t.Errorf("dev user not namespaced: %v", f.users.bySub)
	}
	do(mux, "POST", "/auth/dev", "", `{"name":"bob"}`)
	if len(f.users.bySub) != 1 {
		t.Errorf("same name should reuse the user, got %d users", len(f.users.bySub))
	}
}

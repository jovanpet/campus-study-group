package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jovanpet/campus-study-group/internal/config"
)

const testClientID = "com.example.app"

// fakeApple is an httptest stand-in for appleid.apple.com.
type fakeApple struct {
	t          *testing.T
	srv        *httptest.Server
	signingKey *rsa.PrivateKey
	kid        string
	keyFetches int
	keysStatus int // overrides the /auth/keys status when non-zero

	tokenStatus int
	tokenBody   string
	revoked     []string
	lastForm    map[string]string
}

func newFakeApple(t *testing.T) (*fakeApple, *AppleClient) {
	t.Helper()
	f := &fakeApple{t: t, kid: "key-1", tokenStatus: http.StatusOK, tokenBody: `{"refresh_token":"apple-refresh"}`}
	f.signingKey = mustRSA(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/keys", func(w http.ResponseWriter, _ *http.Request) {
		f.keyFetches++
		if f.keysStatus != 0 {
			w.WriteHeader(f.keysStatus)
			return
		}
		pub := f.signingKey.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": f.kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	form := func(r *http.Request) {
		_ = r.ParseForm()
		f.lastForm = map[string]string{}
		for k := range r.PostForm {
			f.lastForm[k] = r.PostForm.Get(k)
		}
	}
	mux.HandleFunc("POST /auth/token", func(w http.ResponseWriter, r *http.Request) {
		form(r)
		w.WriteHeader(f.tokenStatus)
		_, _ = w.Write([]byte(f.tokenBody))
	})
	mux.HandleFunc("POST /auth/revoke", func(_ http.ResponseWriter, r *http.Request) {
		form(r)
		f.revoked = append(f.revoked, r.PostForm.Get("token"))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	p8 := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	// Escaped newlines, as the key arrives from an env var.
	client, err := NewAppleClient(config.Apple{
		ClientID: testClientID, TeamID: "TEAM123", KeyID: "KEY123",
		PrivateKey: strings.ReplaceAll(p8, "\n", `\n`),
	})
	if err != nil {
		t.Fatalf("NewAppleClient: %v", err)
	}
	client.baseURL = f.srv.URL
	return f, client
}

func mustRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// identityToken signs an Apple-style identity token; mutate adjusts the
// otherwise valid claims.
func (f *fakeApple) identityToken(mutate func(jwt.MapClaims)) string {
	f.t.Helper()
	claims := jwt.MapClaims{
		"iss": appleIssuer, "aud": testClientID, "sub": "apple-user-1",
		"iat": time.Now().Unix(), "exp": time.Now().Add(10 * time.Minute).Unix(),
		"email": "user@privaterelay.appleid.com",
	}
	if mutate != nil {
		mutate(claims)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	s, err := tok.SignedString(f.signingKey)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func TestVerifyIdentityToken(t *testing.T) {
	f, client := newFakeApple(t)
	ctx := context.Background()

	id, err := client.VerifyIdentityToken(ctx, f.identityToken(nil), "")
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if id.Sub != "apple-user-1" || id.Email != "user@privaterelay.appleid.com" {
		t.Errorf("identity = %+v", id)
	}

	sum := sha256.Sum256([]byte("raw-nonce"))
	withNonce := f.identityToken(func(c jwt.MapClaims) { c["nonce"] = hex.EncodeToString(sum[:]) })
	if _, err := client.VerifyIdentityToken(ctx, withNonce, "raw-nonce"); err != nil {
		t.Errorf("matching nonce: %v", err)
	}

	otherKey := mustRSA(t)
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": appleIssuer, "aud": testClientID, "sub": "x", "exp": time.Now().Add(time.Minute).Unix(),
	})
	forged.Header["kid"] = f.kid
	forgedToken, _ := forged.SignedString(otherKey)

	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": appleIssuer, "aud": testClientID, "sub": "x", "exp": time.Now().Add(time.Minute).Unix(),
	})
	hs.Header["kid"] = f.kid
	hsToken, _ := hs.SignedString([]byte("secret"))

	invalid := map[string]struct{ token, nonce string }{
		"wrong audience":  {f.identityToken(func(c jwt.MapClaims) { c["aud"] = "com.other.app" }), ""},
		"wrong issuer":    {f.identityToken(func(c jwt.MapClaims) { c["iss"] = "https://evil.example" }), ""},
		"expired":         {f.identityToken(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }), ""},
		"no expiry":       {f.identityToken(func(c jwt.MapClaims) { delete(c, "exp") }), ""},
		"no subject":      {f.identityToken(func(c jwt.MapClaims) { delete(c, "sub") }), ""},
		"nonce mismatch":  {withNonce, "different"},
		"nonce missing":   {f.identityToken(nil), "raw-nonce"},
		"bad signature":   {forgedToken, ""},
		"wrong algorithm": {hsToken, ""},
		"garbage":         {"not-a-jwt", ""},
	}
	for name, tc := range invalid {
		if _, err := client.VerifyIdentityToken(ctx, tc.token, tc.nonce); !errors.Is(err, ErrInvalidAppleToken) {
			t.Errorf("%s: err = %v, want ErrInvalidAppleToken", name, err)
		}
	}
}

func TestVerifyIdentityTokenKeyCache(t *testing.T) {
	f, client := newFakeApple(t)
	ctx := context.Background()
	now := time.Now()
	client.now = func() time.Time { return now }

	for range 3 {
		if _, err := client.VerifyIdentityToken(ctx, f.identityToken(nil), ""); err != nil {
			t.Fatal(err)
		}
	}
	if f.keyFetches != 1 {
		t.Errorf("key fetches = %d, want 1 (cached)", f.keyFetches)
	}

	// Apple rotates its key: an unknown kid is refetched, but not more than
	// once per jwksMinRefresh.
	f.signingKey, f.kid = mustRSA(t), "key-2"
	if _, err := client.VerifyIdentityToken(ctx, f.identityToken(nil), ""); !errors.Is(err, ErrInvalidAppleToken) {
		t.Errorf("unknown kid inside refresh window: err = %v, want ErrInvalidAppleToken", err)
	}
	now = now.Add(jwksMinRefresh + time.Second)
	if _, err := client.VerifyIdentityToken(ctx, f.identityToken(nil), ""); err != nil {
		t.Errorf("after rotation: %v", err)
	}
	if f.keyFetches != 2 {
		t.Errorf("key fetches = %d, want 2", f.keyFetches)
	}
}

func TestVerifyIdentityTokenAppleDown(t *testing.T) {
	f, client := newFakeApple(t)
	f.keysStatus = http.StatusServiceUnavailable
	if _, err := client.VerifyIdentityToken(context.Background(), f.identityToken(nil), ""); !errors.Is(err, ErrAppleUnavailable) {
		t.Errorf("err = %v, want ErrAppleUnavailable", err)
	}
}

func TestExchangeCodeAndRevoke(t *testing.T) {
	f, client := newFakeApple(t)
	ctx := context.Background()

	refresh, err := client.ExchangeCode(ctx, "auth-code")
	if err != nil || refresh != "apple-refresh" {
		t.Fatalf("ExchangeCode = %q, %v", refresh, err)
	}
	if f.lastForm["grant_type"] != "authorization_code" || f.lastForm["code"] != "auth-code" || f.lastForm["client_id"] != testClientID {
		t.Errorf("token request form = %v", f.lastForm)
	}

	// The client secret must be an ES256 JWT Apple can check against our key.
	claims := jwt.RegisteredClaims{}
	secret, err := jwt.ParseWithClaims(f.lastForm["client_secret"], &claims,
		func(*jwt.Token) (any, error) { return &client.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"ES256"}), jwt.WithAudience(appleIssuer), jwt.WithExpirationRequired())
	if err != nil {
		t.Fatalf("client secret: %v", err)
	}
	if claims.Issuer != "TEAM123" || claims.Subject != testClientID || secret.Header["kid"] != "KEY123" {
		t.Errorf("client secret iss=%q sub=%q kid=%v", claims.Issuer, claims.Subject, secret.Header["kid"])
	}

	f.tokenStatus, f.tokenBody = http.StatusBadRequest, `{"error":"invalid_grant"}`
	if _, err := client.ExchangeCode(ctx, "used-code"); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("used code: err = %v, want ErrInvalidGrant", err)
	}
	f.tokenBody = `{"error":"invalid_client"}`
	if _, err := client.ExchangeCode(ctx, "code"); !errors.Is(err, ErrAppleUnavailable) {
		t.Errorf("invalid_client: err = %v, want ErrAppleUnavailable", err)
	}

	if err := client.Revoke(ctx, "apple-refresh"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "apple-refresh" || f.lastForm["token_type_hint"] != "refresh_token" {
		t.Errorf("revoked = %v, form = %v", f.revoked, f.lastForm)
	}
}

func TestNewAppleClientRejectsBadKey(t *testing.T) {
	if _, err := NewAppleClient(config.Apple{ClientID: "a", TeamID: "b", KeyID: "c", PrivateKey: "nope"}); err == nil {
		t.Error("expected error for a non-PEM key")
	}
}

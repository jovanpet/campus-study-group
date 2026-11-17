package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jovanpet/campus-study-group/internal/config"
)

const (
	appleIssuer = "https://appleid.apple.com"

	jwksTTL = time.Hour
	// jwksMinRefresh limits refetches triggered by tokens with unknown key IDs.
	jwksMinRefresh  = time.Minute
	clientSecretTTL = 5 * time.Minute
)

// Identity is what a verified Apple identity token says about the user.
type Identity struct {
	Sub   string // stable, app-scoped Apple user ID
	Email string // may be empty or a private relay address
}

// AppleClient talks to Apple's Sign in with Apple endpoints.
type AppleClient struct {
	baseURL  string
	clientID string
	teamID   string
	keyID    string
	key      *ecdsa.PrivateKey
	http     *http.Client
	now      func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewAppleClient(cfg config.Apple) (*AppleClient, error) {
	if cfg.ClientID == "" || cfg.TeamID == "" || cfg.KeyID == "" {
		return nil, errors.New("APPLE_CLIENT_ID, APPLE_TEAM_ID and APPLE_KEY_ID are required")
	}
	key, err := parseApplePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("APPLE_PRIVATE_KEY: %w", err)
	}
	return &AppleClient{
		baseURL:  appleIssuer,
		clientID: cfg.ClientID,
		teamID:   cfg.TeamID,
		keyID:    cfg.KeyID,
		key:      key,
		http:     &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
	}, nil
}

// parseApplePrivateKey parses the .p8 file contents. Literal "\n" sequences are
// accepted because env vars often can't hold real newlines.
func parseApplePrivateKey(s string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.ReplaceAll(s, `\n`, "\n")))
	if block == nil {
		return nil, errors.New("not PEM encoded")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an ECDSA key")
	}
	return key, nil
}

type appleClaims struct {
	jwt.RegisteredClaims
	Nonce string `json:"nonce"`
	Email string `json:"email"`
}

// VerifyIdentityToken checks the token's signature against Apple's public
// keys, and its issuer, audience and expiry. If rawNonce is non-empty the
// token's nonce claim must be its hex SHA-256, which is what the app gives
// ASAuthorizationAppleIDRequest.nonce.
func (c *AppleClient) VerifyIdentityToken(ctx context.Context, token, rawNonce string) (Identity, error) {
	var keyErr error
	claims := &appleClaims{}
	_, err := jwt.ParseWithClaims(token, claims,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			key, err := c.publicKey(ctx, kid)
			keyErr = err
			return key, err
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(appleIssuer),
		jwt.WithAudience(c.clientID),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(c.now),
	)
	if errors.Is(keyErr, ErrAppleUnavailable) {
		return Identity{}, keyErr
	}
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrInvalidAppleToken, err)
	}
	if claims.Subject == "" {
		return Identity{}, fmt.Errorf("%w: missing sub", ErrInvalidAppleToken)
	}
	if rawNonce != "" {
		sum := sha256.Sum256([]byte(rawNonce))
		want := hex.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(want), []byte(claims.Nonce)) != 1 {
			return Identity{}, fmt.Errorf("%w: nonce mismatch", ErrInvalidAppleToken)
		}
	}
	return Identity{Sub: claims.Subject, Email: claims.Email}, nil
}

var errUnknownKey = errors.New("unknown signing key")

// publicKey returns Apple's signing key with the given ID, fetching the key
// set when the cache is stale or (rate limited) when the ID is unknown.
func (c *AppleClient) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	age := c.now().Sub(c.fetchedAt)
	key, known := c.keys[kid]
	if c.keys != nil && ((known && age < jwksTTL) || (!known && age < jwksMinRefresh)) {
		if !known {
			return nil, errUnknownKey
		}
		return key, nil
	}

	keys, err := c.fetchKeys(ctx)
	if err != nil {
		if known {
			return key, nil // stale beats unavailable
		}
		return nil, fmt.Errorf("%w: fetch keys: %v", ErrAppleUnavailable, err)
	}
	c.keys, c.fetchedAt = keys, c.now()
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	return nil, errUnknownKey
}

func (c *AppleClient) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/auth/keys", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("no usable keys")
	}
	return keys, nil
}

// clientSecret is the short-lived JWT that authenticates this server to Apple.
func (c *AppleClient) clientSecret() (string, error) {
	now := c.now()
	t := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{
		Issuer:    c.teamID,
		Subject:   c.clientID,
		Audience:  jwt.ClaimStrings{appleIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(clientSecretTTL)),
	})
	t.Header["kid"] = c.keyID
	return t.SignedString(c.key)
}

// ExchangeCode trades the one-time authorization code from the app for an
// Apple refresh token.
func (c *AppleClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	status, body, err := c.post(ctx, "/auth/token", url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
	})
	if err != nil {
		return "", err
	}
	var res struct {
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(body, &res)
	switch {
	case status == http.StatusOK && res.RefreshToken != "":
		return res.RefreshToken, nil
	case status == http.StatusBadRequest && res.Error == "invalid_grant":
		return "", ErrInvalidGrant
	default:
		return "", fmt.Errorf("%w: token endpoint returned %d %q", ErrAppleUnavailable, status, res.Error)
	}
}

// Revoke invalidates the refresh token, removing the app from the user's
// Apple ID. Apple answers 200 for tokens that are already invalid.
func (c *AppleClient) Revoke(ctx context.Context, refreshToken string) error {
	status, _, err := c.post(ctx, "/auth/revoke", url.Values{
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%w: revoke endpoint returned %d", ErrAppleUnavailable, status)
	}
	return nil
}

func (c *AppleClient) post(ctx context.Context, path string, form url.Values) (int, []byte, error) {
	secret, err := c.clientSecret()
	if err != nil {
		return 0, nil, fmt.Errorf("sign apple client secret: %w", err)
	}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrAppleUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrAppleUnavailable, err)
	}
	return resp.StatusCode, body, nil
}

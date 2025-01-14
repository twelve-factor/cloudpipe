package cmd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

type idToken struct {
	Issuer string `json:"iss"`
}

var (
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
)

func generateToken(issuer string, audience string, subject string) (string, error) {
	claims := jwt.MapClaims{
		"iss": issuer,
		"aud": audience,
		"sub": subject,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %v", err)
	}
	log.Infof("Generated token: %s", signedToken)

	return signedToken, nil
}

func registerOIDCRoutes(api *http.ServeMux) {
	var err error
	privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}
	publicKey = &privateKey.PublicKey

	api.Handle("/.well-known/authorize", http.HandlerFunc(notImplementedHandler))
	api.Handle("/.well-known/token", http.HandlerFunc(notImplementedHandler))
	api.Handle("/.well-known/openid-configuration", http.HandlerFunc(openIDConfigHandler))
	api.Handle("/.well-known/jwks.json", http.HandlerFunc(jwksHandler))
}

func notImplementedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Endpoint is not implemented", http.StatusNotImplemented)
}

func openIDConfigHandler(w http.ResponseWriter, r *http.Request) {
	sc := r.Context().Value(configKey).(ServerConfig)
	config := map[string]interface{}{
		"issuer":                                sc.Prefix,
		"authorization_endpoint":                sc.Prefix + "/authorize",
		"token_endpoint":                        sc.Prefix + "/token",
		"jwks_uri":                              sc.Prefix + "/.well-known/jwks.json",
		"response_types_supported":              []string{},
		"grant_types_supported":                 []string{},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func jwksHandler(w http.ResponseWriter, r *http.Request) {
	n := base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes())
	jwk := map[string]interface{}{
		"kty": "RSA",
		"kid": "example-key-id",
		"use": "sig",
		"alg": "RS256",
		"n":   n,
		"e":   e,
	}

	keys := map[string]interface{}{
		"keys": []interface{}{jwk},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func getIssuer(p string) (string, error) {
	log.Infof("Getting issuer from token: %s", p)
	parts := strings.Split(p, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("malformed jwt, expected 3 parts got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("malformed jwt payload: %w", err)
	}

	var token idToken
	if err := json.Unmarshal(payload, &token); err != nil {
		return "", fmt.Errorf("failed to unmarshal token: %w", err)
	}
	return token.Issuer, nil
}

type Validator struct {
	Iss *regexp.Regexp
	Aud *regexp.Regexp
	Sub *regexp.Regexp
}

func Validate(ctx context.Context, rawToken string, val Validator) error {
	issuer, err := getIssuer(rawToken)
	if err != nil {
		return err
	}

	if !val.Iss.MatchString(issuer) {
		return fmt.Errorf("unmatched issuer: %v %v", issuer, val.Iss)
	}

	// TODO: cache providers
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	var verifier = provider.Verifier(&oidc.Config{SkipClientIDCheck: true})

	token, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	match := false
	for _, aud := range token.Audience {
		if val.Aud.MatchString(aud) {
			match = true
			break
		}
	}
	if !match {
		return fmt.Errorf("unmatched audience: %v %v", token.Audience, val)
	}

	results := val.Sub.FindStringSubmatch(token.Subject)
	if results == nil {
		return fmt.Errorf("unmatched subject: %v %v", token.Subject, val)
	}
	return nil
}

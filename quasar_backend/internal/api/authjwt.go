package api

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
)

type userJWTClaims struct {
	UserID string `json:"uid"` // UUID em texto
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func mintUserJWT(cfg *config.Config, userID uuid.UUID, email, role string) (string, error) {
	claims := userJWTClaims{
		UserID: userID.String(),
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(48 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "netquasar",
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(cfg.JWTSigningSecret())
}

// parseUserJWT valida assinatura e expiração; devolve dados do token.
func parseUserJWT(cfg *config.Config, raw string) (userID uuid.UUID, email, role string, err error) {
	if raw == "" {
		return uuid.Nil, "", "", errors.New("token vazio")
	}
	var claims userJWTClaims
	_, err = jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inesperado: %v", t.Header["alg"])
		}
		return cfg.JWTSigningSecret(), nil
	})
	if err != nil {
		return uuid.Nil, "", "", err
	}
	if claims.UserID == "" || claims.Email == "" {
		return uuid.Nil, "", "", errors.New("claims inválidos")
	}
	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, "", "", err
	}
	return uid, claims.Email, claims.Role, nil
}

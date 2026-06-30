package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWT struct {
	secret []byte
}

func NewJWT(secret []byte) *JWT {
	return &JWT{secret: secret}
}

func (j *JWT) Sign(claims map[string]any, expiry time.Duration) (string, error) {
	jwtClaims := jwt.MapClaims(claims)
	jwtClaims["iat"] = time.Now().Unix()
	jwtClaims["exp"] = time.Now().Add(expiry).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString(j.secret)
}

func (j *JWT) Verify(token string) (map[string]any, error) {
	parsed, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	return map[string]any(claims), nil
}

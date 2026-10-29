package utils

import (
	"time"

	"automation-developer-guide/src/config"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateJWT(userID, email, username, avatarURL, firstName, lastName string) (string, error) {
	return GenerateJWTWithExtra(userID, email, username, avatarURL, firstName, lastName, nil)
}

// GenerateJWTWithExtra adds claims (e.g. the ONDC organisation) on top of the
// base set; extra keys never override the base claims.
func GenerateJWTWithExtra(userID, email, username, avatarURL, firstName, lastName string, extra map[string]interface{}) (string, error) {
	claims := jwt.MapClaims{}
	for k, v := range extra {
		claims[k] = v
	}
	claims["user_id"] = userID
	claims["role"] = "AUDITOR"
	claims["email"] = email
	claims["username"] = username
	claims["avatar_url"] = avatarURL
	claims["first_name"] = firstName
	claims["last_name"] = lastName
	claims["exp"] = time.Now().Add(8 * time.Hour).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.JWTSecret))
}

func ParseJWT(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, err
}

package ginboot

import (
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"os"
	"time"
)

type Claims struct {
	Role string `json:"role"`
	jwt.StandardClaims
}

func GenerateTokens(userId string, role string) (string, string, error) {
	var tokenSecret = os.Getenv("JWT_SECRET")
	accessToken, err := generateJwtToken(userId, role, 24*time.Hour, tokenSecret)
	if err != nil {
		return "", "", err
	}

	var refreshTokenSecret = os.Getenv("JWT_REFRESH_SECRET")
	refreshToken, err := generateJwtToken(userId, role, 24*30*time.Hour, refreshTokenSecret)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func generateJwtToken(userId string, role string, duration time.Duration, secretKey string) (string, error) {
	var jwtKeyBytes = []byte(secretKey)
	expirationTime := time.Now().Add(duration)
	claims := &Claims{
		Role: role,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
			Id:        uuid.New().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "klass-lk",
			Subject:   userId,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKeyBytes)
}

func ParseAccessToken(tokenString string) (*jwt.Token, error) {
	var tokenSecret = os.Getenv("JWT_SECRET")
	return parseJwtToken(tokenString, tokenSecret)
}
func ParseRefreshToken(tokenString string) (*jwt.Token, error) {
	var tokenSecret = os.Getenv("JWT_REFRESH_SECRET")
	return parseJwtToken(tokenString, tokenSecret)
}

func parseJwtToken(tokenString string, secretKey string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	return token, err
}

func ExtractClaims(token *jwt.Token) (jwt.MapClaims, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("bearer token is invalid")
	}
	return claims, nil
}

func IsExpired(claims jwt.MapClaims) bool {
	return float64(time.Now().Unix()) > claims["exp"].(float64)
}

func ExtractUserId(claims jwt.MapClaims) string {
	return claims["sub"].(string)
}

func ExtractRole(claims jwt.MapClaims) string {
	return claims["role"].(string)
}

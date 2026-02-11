package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/bekci/internal/store"
)

type Service struct {
	store     *store.Store
	jwtSecret []byte
}

// Claims is the JWT payload.
type Claims struct {
	SessionID string `json:"sid"`
	Role      string `json:"role"`
	jwt.RegisteredClaims
}

func New(s *store.Store, jwtSecret string) *Service {
	return &Service{store: s, jwtSecret: []byte(jwtSecret)}
}

// HashPassword hashes a plaintext password with bcrypt cost 12.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

// CheckPassword verifies a password against a bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CreateToken generates a signed JWT for the given session.
func (svc *Service) CreateToken(userID, sessionID, role string, duration time.Duration) (string, error) {
	claims := Claims{
		SessionID: sessionID,
		Role:      role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(svc.jwtSecret)
}

// ValidateToken parses and validates a JWT, checks the session exists and is not expired.
func (svc *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return svc.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Verify session exists and not expired
	sess, err := svc.store.GetSession(claims.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session lookup: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found (logged out)")
	}
	if time.Now().After(sess.ExpiresAt) {
		svc.store.DeleteSession(sess.ID)
		return nil, fmt.Errorf("session expired")
	}

	return claims, nil
}

// Login authenticates a user and creates a session + JWT.
func (svc *Service) Login(username, password, ipAddress string) (string, *store.User, error) {
	user, err := svc.store.GetUserByUsername(username)
	if err != nil {
		return "", nil, fmt.Errorf("database error: %w", err)
	}
	if user == nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}
	if user.Status != "active" {
		return "", nil, fmt.Errorf("account suspended")
	}
	if !CheckPassword(user.PasswordHash, password) {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	// Get session timeout from settings
	timeoutStr, err := svc.store.GetSetting("session_timeout_hours")
	if err != nil || timeoutStr == "" {
		timeoutStr = "24"
	}
	hours, _ := strconv.Atoi(timeoutStr)
	if hours < 1 {
		hours = 24
	}
	duration := time.Duration(hours) * time.Hour

	sessionID := uuid.New().String()
	sess := &store.Session{
		ID:        sessionID,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(duration),
		IPAddress: ipAddress,
		CreatedAt: time.Now(),
	}
	if err := svc.store.CreateSession(sess); err != nil {
		return "", nil, fmt.Errorf("creating session: %w", err)
	}

	token, err := svc.CreateToken(user.ID, sessionID, user.Role, duration)
	if err != nil {
		return "", nil, fmt.Errorf("creating token: %w", err)
	}

	return token, user, nil
}

// Logout deletes a session.
func (svc *Service) Logout(sessionID string) error {
	return svc.store.DeleteSession(sessionID)
}

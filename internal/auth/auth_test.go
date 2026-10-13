package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/bekci/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func createTestUser(t *testing.T, st *store.Store, username, password, role string) *store.User {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Status:       "active",
	}
	if err := st.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("mysecretpassword")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "mysecretpassword") {
		t.Fatal("expected CheckPassword to return true for correct password")
	}
	if CheckPassword(hash, "wrongpassword") {
		t.Fatal("expected CheckPassword to return false for wrong password")
	}
}

func TestCreateAndValidateToken(t *testing.T) {
	st := newTestStore(t)
	svc := New(st, "testsecret")

	user := createTestUser(t, st, "tokenuser", "pass123", "admin")

	sessionID := uuid.New().String()
	sess := &store.Session{
		ID:        sessionID,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
		IPAddress: "127.0.0.1",
		CreatedAt: time.Now(),
	}
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	token, err := svc.CreateToken(user.ID, sessionID, "admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != user.ID {
		t.Fatalf("expected Subject %s, got %s", user.ID, claims.Subject)
	}
	if claims.SessionID != sessionID {
		t.Fatalf("expected SessionID %s, got %s", sessionID, claims.SessionID)
	}
	if claims.Role != "admin" {
		t.Fatalf("expected Role admin, got %s", claims.Role)
	}
}

func TestValidateTokenExpiredSession(t *testing.T) {
	st := newTestStore(t)
	svc := New(st, "testsecret")

	user := createTestUser(t, st, "expuser", "pass123", "admin")

	sessionID := uuid.New().String()
	sess := &store.Session{
		ID:        sessionID,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
		IPAddress: "127.0.0.1",
		CreatedAt: time.Now(),
	}
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	token, err := svc.CreateToken(user.ID, sessionID, "admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteSession(sessionID); err != nil {
		t.Fatal(err)
	}

	_, err = svc.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for deleted session, got nil")
	}
}

// Review item D-25: only tokens shaped like the ones CreateToken issues are
// accepted — HS256, an exp claim, and a sub that owns the session.
func TestValidateTokenRejectsTokensNotIssuedByCreateToken(t *testing.T) {
	st := newTestStore(t)
	svc := New(st, "testsecret")

	user := createTestUser(t, st, "shapeuser", "pass123", "admin")
	other := createTestUser(t, st, "otheruser", "pass123", "viewer")

	sessionID := uuid.New().String()
	if err := st.CreateSession(&store.Session{
		ID:        sessionID,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
		IPAddress: "127.0.0.1",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	sign := func(method jwt.SigningMethod, sub string, exp *jwt.NumericDate) string {
		t.Helper()
		claims := Claims{
			SessionID: sessionID,
			Role:      "admin",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   sub,
				ExpiresAt: exp,
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		s, err := jwt.NewWithClaims(method, claims).SignedString([]byte("testsecret"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	inAnHour := jwt.NewNumericDate(time.Now().Add(time.Hour))

	cases := []struct {
		name   string
		token  string
		wantOK bool
	}{
		{"HS256 with exp and the session's user", sign(jwt.SigningMethodHS256, user.ID, inAnHour), true},
		{"HS384 with the right secret", sign(jwt.SigningMethodHS384, user.ID, inAnHour), false},
		{"HS512 with the right secret", sign(jwt.SigningMethodHS512, user.ID, inAnHour), false},
		{"no exp claim", sign(jwt.SigningMethodHS256, user.ID, nil), false},
		{"sub is another user than the session's", sign(jwt.SigningMethodHS256, other.ID, inAnHour), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ValidateToken(tc.token)
			if tc.wantOK && err != nil {
				t.Fatalf("expected token to be accepted, got %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("expected token to be rejected, got nil error")
			}
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	st := newTestStore(t)
	svc := New(st, "testsecret")

	createTestUser(t, st, "loginuser", "correctpass", "operator")

	token, user, duration, err := svc.Login("loginuser", "correctpass", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if user.Username != "loginuser" {
		t.Fatalf("expected username loginuser, got %s", user.Username)
	}
	if duration <= 0 {
		t.Fatalf("expected positive duration, got %v", duration)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("expected token to be valid, got error: %v", err)
	}
	if claims.Subject != user.ID {
		t.Fatalf("expected Subject %s, got %s", user.ID, claims.Subject)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	st := newTestStore(t)
	svc := New(st, "testsecret")

	createTestUser(t, st, "wronguser", "realpass", "viewer")

	token, _, _, err := svc.Login("wronguser", "badpass", "127.0.0.1")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	if token != "" {
		t.Fatalf("expected empty token, got %s", token)
	}
}

func TestLoginSuspendedUser(t *testing.T) {
	st := newTestStore(t)
	svc := New(st, "testsecret")

	user := createTestUser(t, st, "suspuser", "pass123", "operator")

	if err := st.SuspendUser(user.ID, true); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := svc.Login("suspuser", "pass123", "127.0.0.1")
	if err == nil {
		t.Fatal("expected error for suspended user, got nil")
	}
	if !strings.Contains(err.Error(), "suspended") {
		t.Fatalf("expected error to contain 'suspended', got: %s", err.Error())
	}
}

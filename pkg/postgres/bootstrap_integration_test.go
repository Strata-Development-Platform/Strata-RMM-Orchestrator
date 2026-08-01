//go:build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func TestBootstrapInitialAdminExactlyOnce(t *testing.T) {
	base := os.Getenv("TEST_POSTGRES_DSN")
	if base == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	dbName := fmt.Sprintf("bootstrap_%d", time.Now().UnixNano())
	controlURL := *parsed
	controlURL.Path = "/postgres"
	control, err := sql.Open("postgres", controlURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	if _, err := control.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = control.Exec("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)")
	}()

	testURL := *parsed
	testURL.Path = "/" + dbName
	db, err := sql.Open("postgres", testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := NewSchemaManager(db).Apply(ctx); err != nil {
		t.Fatal(err)
	}

	input := BootstrapAdminInput{
		Email:      "owner@example.com",
		Password:   "correct horse battery staple",
		TenantName: "Example Platform",
	}
	userID, err := BootstrapInitialAdmin(ctx, db, input)
	if err != nil {
		t.Fatal(err)
	}
	if userID == "" {
		t.Fatal("bootstrap returned an empty user ID")
	}

	var email, role, hash string
	if err := db.QueryRow("SELECT email, role, password_hash FROM users WHERE id = $1", userID).Scan(&email, &role, &hash); err != nil {
		t.Fatal(err)
	}
	if email != "owner@example.com" || role != "admin" {
		t.Fatalf("unexpected initial user: email=%q role=%q", email, role)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)); err != nil {
		t.Fatal("stored password hash does not match")
	}

	var membershipRole, scopeType, scopeID, membershipStatus string
	if err := db.QueryRow(`
		SELECT role, scope_type, scope_id, status
		FROM memberships
		WHERE user_id = $1
	`, userID).Scan(&membershipRole, &scopeType, &scopeID, &membershipStatus); err != nil {
		t.Fatal(err)
	}
	if membershipRole != "platform_owner" || scopeType != "platform" ||
		scopeID != SingletonPlatformID || membershipStatus != "active" {
		t.Fatalf("unexpected bootstrap membership: role=%q type=%q id=%q status=%q",
			membershipRole, scopeType, scopeID, membershipStatus)
	}

	if _, err := BootstrapInitialAdmin(ctx, db, input); !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("second bootstrap error = %v, want ErrAlreadyBootstrapped", err)
	}

	var auditCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action = 'platform.bootstrap_admin'").Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("bootstrap audit events = %d, want 1", auditCount)
	}
}

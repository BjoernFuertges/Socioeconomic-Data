package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestBuildConnStringSetsStatementTimeout(t *testing.T) {
	got := buildConnString("db", "5432", "u", "p", "soziodata", "disable")
	for _, want := range []string{
		"host='db'", "port='5432'", "dbname='soziodata'", "sslmode='disable'",
		"options='-c statement_timeout=60000'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conn string %q lacks %q", got, want)
		}
	}
}

func TestDSNValueEscaping(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", `'plain'`},
		{"", `''`},
		{"p ss", `'p ss'`},
		{`p ss'\x`, `'p ss\'\\x'`},
		{`a=b c='d'`, `'a=b c=\'d\''`},
	}
	for _, tt := range tests {
		if got := dsnValue(tt.in); got != tt.want {
			t.Errorf("dsnValue(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

// A password with space, quote and backslash must neither break the DSN nor
// shift the following keys (dbname=... must stay its own key).
func TestBuildConnStringSpecialPasswordParses(t *testing.T) {
	dsn := buildConnString("db", "5432", "u", `p ss'\x`, "soziodata", "disable")
	if _, err := pq.NewConnector(dsn); err != nil {
		t.Fatalf("lib/pq rejects %q: %v", dsn, err)
	}
	if !strings.Contains(dsn, `password='p ss\'\\x'`) {
		t.Errorf("password not escaped: %s", dsn)
	}
	// Without quoting, an empty value would swallow the next key.
	if _, err := pq.NewConnector(buildConnString("db", "5432", "u", "", "soziodata", "disable")); err != nil {
		t.Errorf("empty password: %v", err)
	}
}

// TestBuildConnStringConnects logs in with the special password against a
// real Postgres. Opt-in: set SOZ_TEST_PG_HOST/PORT/USER/PASSWORD/DB (e.g. a
// throwaway postgis container started with POSTGRES_PASSWORD="p ss'\x").
func TestBuildConnStringConnects(t *testing.T) {
	host := os.Getenv("SOZ_TEST_PG_HOST")
	if host == "" {
		t.Skip("SOZ_TEST_PG_HOST not set")
	}
	dsn := buildConnString(host, os.Getenv("SOZ_TEST_PG_PORT"), os.Getenv("SOZ_TEST_PG_USER"),
		os.Getenv("SOZ_TEST_PG_PASSWORD"), os.Getenv("SOZ_TEST_PG_DB"), "disable")
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var timeout string
	if err := db.GetContext(ctx, &timeout, "SHOW statement_timeout"); err != nil {
		t.Fatal(err)
	}
	if timeout != "1min" {
		t.Errorf("statement_timeout = %q, want 1min", timeout)
	}
}

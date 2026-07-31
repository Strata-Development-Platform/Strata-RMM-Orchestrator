package backup

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecretPostgreSQLCommandRedaction(t *testing.T) {
	for _, dsn := range []string{
		"postgres://operator:super-secret@db.internal:5432/strata?sslmode=require",
		"host=db.internal user=operator password=super-secret dbname=strata sslmode=require",
	} {
		err := sanitizedCommandError("pg_dump", errors.New("exit status 1"), "connection failed: "+dsn, dsn)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "super-secret")
		require.NotContains(t, strings.ToLower(err.Error()), "password=super-secret")
	}
}

func TestPostgresCommandEnvKeepsCredentialsOutOfArguments(t *testing.T) {
	environment, err := postgresCommandEnv("postgres://operator:super-secret@db.internal:5433/strata?sslmode=verify-full")
	require.NoError(t, err)
	joined := strings.Join(environment, "\n")
	require.Contains(t, joined, "PGHOST=db.internal")
	require.Contains(t, joined, "PGPORT=5433")
	require.Contains(t, joined, "PGDATABASE=strata")
	require.Contains(t, joined, "PGPASSWORD=super-secret")
	require.Contains(t, joined, "PGSSLMODE=verify-full")
	require.NotContains(t, strings.Join(os.Args, " "), "super-secret")

	environment, err = postgresCommandEnv("host=db.internal port=5432 user=operator password='space secret' dbname=strata sslmode=require")
	require.NoError(t, err)
	joined = strings.Join(environment, "\n")
	require.Contains(t, joined, "PGPASSWORD=space secret")
	require.Contains(t, joined, "PGDATABASE=strata")
}

func TestPostgreSQLRecoveryRejectsSameTarget(t *testing.T) {
	dsn := "postgres://operator:secret@db.internal/strata"
	component, err := NewPostgreSQLRecovery(dsn, dsn)
	require.Error(t, err)
	require.Nil(t, component)
}

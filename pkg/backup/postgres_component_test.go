package backup

import (
	"errors"
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

func TestPostgreSQLRecoveryRejectsSameTarget(t *testing.T) {
	dsn := "postgres://operator:secret@db.internal/strata"
	component, err := NewPostgreSQLRecovery(dsn, dsn)
	require.Error(t, err)
	require.Nil(t, component)
}

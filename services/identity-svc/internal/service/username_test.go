package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUsername(t *testing.T) {
	for _, bad := range []string{"", " ", "a", "bad@name", "has/slash", "sp ace"} {
		_, err := validateUsername(bad)
		require.Error(t, err, bad)
		var verr *ValidationError
		assert.ErrorAs(t, err, &verr, bad)
	}
	got, err := validateUsername("  ok_name-1 ")
	require.NoError(t, err)
	assert.Equal(t, "ok_name-1", got)
}

func TestSignupRejectsInvalidUsernameBeforeTouchingDB(t *testing.T) {
	svc := &Service{}
	_, err := svc.Signup(t.Context(), SignupRequest{AccountName: "A", Email: "a@example.com", Password: "pw", Username: "bad@name"})
	var verr *ValidationError
	require.ErrorAs(t, err, &verr)
}

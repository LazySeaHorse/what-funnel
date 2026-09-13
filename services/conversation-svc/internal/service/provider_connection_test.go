package service

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsProviderConnectionLabelConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matching unique index",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "idx_channels_account_provider_label"},
			want: true,
		},
		{
			name: "different unique index",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "other_index"},
		},
		{
			name: "different database error",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "idx_channels_account_provider_label"},
		},
		{
			name: "wrapped matching error",
			err:  errors.Join(errors.New("insert channel"), &pgconn.PgError{Code: "23505", ConstraintName: "idx_channels_account_provider_label"}),
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isProviderConnectionLabelConflict(test.err); got != test.want {
				t.Errorf("isProviderConnectionLabelConflict() = %t, want %t", got, test.want)
			}
		})
	}
}

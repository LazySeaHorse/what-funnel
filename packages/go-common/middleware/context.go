package middleware

import (
	"context"
)

// contextKey is the internal type for context keys in this package.
type contextKey string

// withValue stores a value in context using a typed key to avoid collisions.
func withValue(ctx context.Context, key any, value any) context.Context {
	return context.WithValue(ctx, key, value)
}

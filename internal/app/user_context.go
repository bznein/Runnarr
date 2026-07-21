package app

import (
	"context"
	"errors"
)

var ErrUserContextMissing = errors.New("user context is missing")

type userContextKey struct{}

type UserPrincipal struct {
	ID            string
	Username      string
	DisplayName   string
	Role          string
	SupportTarget bool
	ActorID       string
}

func withPrincipal(ctx context.Context, principal UserPrincipal) context.Context {
	return context.WithValue(ctx, userContextKey{}, principal)
}

func principalFromContext(ctx context.Context) (UserPrincipal, error) {
	principal, ok := ctx.Value(userContextKey{}).(UserPrincipal)
	if !ok || principal.ID == "" {
		return UserPrincipal{}, ErrUserContextMissing
	}
	return principal, nil
}

func userIDFromContext(ctx context.Context) (string, error) {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return "", err
	}
	return principal.ID, nil
}

func withUserID(ctx context.Context, userID string) context.Context {
	return withPrincipal(ctx, UserPrincipal{ID: userID, ActorID: userID})
}

func supportModeFromContext(ctx context.Context) bool {
	principal, err := principalFromContext(ctx)
	return err == nil && principal.SupportTarget
}

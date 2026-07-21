package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrOIDCIdentityConflict = errors.New("OIDC identity is already linked to another user")

func hashOIDCValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateOIDCAuthState(ctx context.Context, state, nonce string, ttl time.Duration) error {
	_, err := s.db.Exec(ctx, `delete from oidc_auth_states where expires_at <= now()`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		insert into oidc_auth_states(state_hash, nonce_hash, expires_at)
		values($1, $2, now() + $3::interval)
	`, hashOIDCValue(state), hashOIDCValue(nonce), fmt.Sprintf("%d seconds", int(ttl.Seconds())))
	return err
}

func (s *Store) ConsumeOIDCAuthState(ctx context.Context, state string) (string, error) {
	var nonceHash string
	err := s.db.QueryRow(ctx, `
		delete from oidc_auth_states
		where state_hash = $1 and expires_at > now()
		returning nonce_hash
	`, hashOIDCValue(state)).Scan(&nonceHash)
	return nonceHash, err
}

func (s *Store) OIDCIdentity(ctx context.Context, issuer, subject string) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `
		select user_id::text from oidc_identities where issuer = $1 and subject = $2
	`, strings.TrimSpace(issuer), strings.TrimSpace(subject)).Scan(&userID)
	return userID, err
}

func (s *Store) LinkOIDCIdentity(ctx context.Context, userID, issuer, subject, email string) error {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	email = strings.ToLower(strings.TrimSpace(email))
	var existingUserID string
	err := s.db.QueryRow(ctx, `
		select user_id::text from oidc_identities where issuer = $1 and subject = $2
	`, issuer, subject).Scan(&existingUserID)
	if err == nil {
		if existingUserID != userID {
			return ErrOIDCIdentityConflict
		}
		_, err = s.db.Exec(ctx, `update oidc_identities set email = $3, updated_at = now() where issuer = $1 and subject = $2`, issuer, subject, email)
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = s.db.Exec(ctx, `
		insert into oidc_identities(user_id, issuer, subject, email)
		values($1, $2, $3, $4)
		on conflict do nothing
	`, userID, issuer, subject, email)
	if err != nil {
		return err
	}
	err = s.db.QueryRow(ctx, `
		select user_id::text from oidc_identities where issuer = $1 and subject = $2
	`, issuer, subject).Scan(&existingUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOIDCIdentityConflict
	}
	if err != nil {
		return err
	}
	if existingUserID != userID {
		return ErrOIDCIdentityConflict
	}
	return nil
}

package auth

import (
	"context"
	"errors"
	"time"
)

var ErrNoSession = errors.New("no session")

// EnsureSession returns a valid session. If the access token is expired (or close),
// it refreshes it using the refresh token.
func EnsureSession(ctx context.Context, oauth SupabaseOAuth) (Session, error) {
	s, ok, err := LoadSession()
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, ErrNoSession
	}

	if !s.NeedsRefresh(time.Now().UTC()) {
		return s, nil
	}
	if s.RefreshToken == "" {
		return Session{}, ErrNoSession
	}

	tr, err := oauth.Refresh(ctx, s.RefreshToken)
	if err != nil {
		return Session{}, err
	}

	next := Session{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresIn:    tr.ExpiresIn,
	}
	if err := SaveSession(next); err != nil {
		return Session{}, err
	}
	return next, nil
}

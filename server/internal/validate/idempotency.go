package validate

import (
	"errors"
	"regexp"
	"strings"
)

const maxIdempotencyKeyLen = 80

var idempotencyKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

func IdempotencyKey(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("missing idempotency key")
	}
	if len(s) > maxIdempotencyKeyLen {
		return errors.New("idempotency key too long")
	}
	if !idempotencyKeyRe.MatchString(s) {
		return errors.New("invalid idempotency key")
	}
	return nil
}

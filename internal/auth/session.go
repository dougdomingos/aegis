package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/errors"
)

// SignSessionToken builds an HMAC-SHA256-signed session token encoding the
// machine ID and its expiry, and returns the token along with the expiry
// timestamp.
func SignSessionToken(machineID int64, secret string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(ttl)
	payload := fmt.Sprintf("%d:%d", machineID, expiresAt.Unix())

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))

	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return token, expiresAt, nil
}

// ParseSessionToken verifies the provided session token against the signing
// secret and the current time, returning the machine ID it encodes. Any
// failure is reported as an error wrapping errors.ErrSessionTokenInvalid.
func ParseSessionToken(token string, secret string, now time.Time) (int64, error) {
	invalid := func(reason string) (int64, error) {
		return 0, fmt.Errorf("%w: %s", errors.ErrSessionTokenInvalid, reason)
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return invalid("token must have exactly two segments")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return invalid("payload segment is not valid base64url")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return invalid("signature segment is not valid base64url")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), signature) {
		return invalid("signature does not match the payload")
	}

	machineID, expiresAtUnix, ok := strings.Cut(string(payload), ":")
	if !ok {
		return invalid("payload is missing the expiry separator")
	}

	id, err := strconv.ParseInt(machineID, 10, 64)
	if err != nil {
		return invalid("payload machine id is not numeric")
	}

	expiry, err := strconv.ParseInt(expiresAtUnix, 10, 64)
	if err != nil {
		return invalid("payload expiry is not numeric")
	}

	if now.Unix() >= expiry {
		return invalid("token is expired")
	}

	return id, nil
}

package n8n

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// IdempotencyKeyHeader is the request header n8n reads a retry key from.
const IdempotencyKeyHeader = "Idempotency-Key"

// MaxIdempotencyKeyLength is the longest key the API accepts.
const MaxIdempotencyKeyLength = 128

// CallOption adjusts one API call without changing its body. Only methods that
// take a variadic CallOption accept one.
type CallOption func(*Request)

// IdempotencyKey makes a POST safe to retry. n8n runs the first request sent
// with key and answers every later request with the same key and the same
// method, path, query and body with the first successful response, without
// running it again. The same key with a different request is refused with 422
// ([IsIdempotencyMismatch]); a retry while the first request still runs is
// refused with 409 ([IsIdempotencyInProgress]). A failed request releases the
// key, so the same key runs it again.
//
// The client never retries on its own: the caller decides when to send the
// request again, with the same key and the same body.
func IdempotencyKey(key string) CallOption {
	return func(r *Request) { r.IdempotencyKey = key }
}

// ValidateIdempotencyKey checks the format the API accepts: 1 to 128 visible
// ASCII characters, so no spaces, control characters or non-ASCII text.
func ValidateIdempotencyKey(key string) error {
	if key == "" {
		return errors.New("idempotency key must not be empty")
	}
	if len(key) > MaxIdempotencyKeyLength {
		return fmt.Errorf("idempotency key must contain at most %d characters, got %d", MaxIdempotencyKeyLength, len(key))
	}
	for i := 0; i < len(key); i++ {
		if c := key[i]; c < 0x21 || c > 0x7e {
			return fmt.Errorf("idempotency key must contain only visible ASCII characters (no spaces): invalid byte %q at position %d", c, i+1)
		}
	}
	return nil
}

// NewIdempotencyKey returns a random version 4 UUID, for callers that have no
// natural key of their own.
func NewIdempotencyKey() string {
	var b [16]byte
	// crypto/rand.Read never fails and always fills the buffer.
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 9562 variant
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// IsIdempotencyInProgress reports the 409 n8n answers while the first request
// sent with the same Idempotency-Key is still running. Retry later with the
// same key. Other 409s (a resource conflict) are not matched.
func IsIdempotencyInProgress(err error) bool {
	return isIdempotencyRefusal(err, http.StatusConflict)
}

// IsIdempotencyMismatch reports the 422 n8n answers when an Idempotency-Key was
// already used with a different request. Send it with a new key. Other 422s
// (an invalid request) are not matched.
func IsIdempotencyMismatch(err error) bool {
	return isIdempotencyRefusal(err, http.StatusUnprocessableEntity)
}

func isIdempotencyRefusal(err error, status int) bool {
	apiErr, ok := errors.AsType[*APIError](err)
	return ok && apiErr.StatusCode == status && mentionsIdempotencyKey(apiErr.Message)
}

// mentionsIdempotencyKey tells the refusals the idempotency layer sends, which
// name the header, from the resource conflicts the same status codes carry.
func mentionsIdempotencyKey(message string) bool {
	return strings.Contains(strings.ToLower(message), strings.ToLower(IdempotencyKeyHeader))
}

// isIdempotencyRefusalBody reports whether a raw error body is an
// idempotency refusal rather than the endpoint's own error document.
func isIdempotencyRefusalBody(raw []byte) bool {
	var body struct {
		Message string `json:"message"`
	}
	return json.Unmarshal(raw, &body) == nil && mentionsIdempotencyKey(body.Message)
}

// applyCallOptions folds opts into r.
func applyCallOptions(r *Request, opts []CallOption) {
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
}

// idempotentBoundary derives a multipart boundary from an idempotency key.
// n8n fingerprints the raw body, so a retry has to send the same bytes, and a
// random boundary would turn every retry into a 422. The hash keeps the
// boundary from occurring in the package bytes by chance.
func idempotentBoundary(key string) string {
	sum := sha256.Sum256([]byte("n8n-cli multipart boundary\x00" + key))
	return "n8n-cli-" + hex.EncodeToString(sum[:24])
}

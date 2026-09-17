// Package n8n is the typed client for the n8n public API.
//
// This file holds the transport every resource method shares: URL building,
// header injection, JSON encoding and decoding, structured errors and debug
// logging. Resource files add request and response models next to the method
// that uses them; none of them build URLs or handle status codes themselves.
package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout bounds a single request when the caller sets no deadline.
const DefaultTimeout = 30 * time.Second

const contentTypeJSON = "application/json"

// Doer is the subset of *http.Client the transport needs. Tests inject their
// own; production code passes a real client.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client talks to one n8n instance with one credential.
type Client struct {
	base      *url.URL
	http      Doer
	auth      Authenticator
	userAgent string
	timeout   time.Duration
	logger    *slog.Logger
}

// Option configures a [Client].
type Option func(*Client)

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(d Doer) Option {
	return func(c *Client) {
		if d != nil {
			c.http = d
		}
	}
}

// WithAuth sets the credential injected into every request. A client without
// one sends no credential, which the API answers with 401 for most operations.
func WithAuth(a Authenticator) Option {
	return func(c *Client) { c.auth = a }
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithTimeout bounds each request. Zero or negative disables the client-side
// timeout and leaves cancellation entirely to the caller's context.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithLogger sets the debug logger. Requests log method, URL, status and
// duration; credentials never reach it, because headers are never logged and
// [Secret] redacts itself.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}

// New normalizes the instance URL and builds a client for it.
func New(instanceURL string, opts ...Option) (*Client, error) {
	base, err := NormalizeBaseURL(instanceURL)
	if err != nil {
		return nil, err
	}
	return NewWithBaseURL(base, opts...), nil
}

// NewWithBaseURL builds a client from an already normalized base URL, as
// returned by [NormalizeBaseURL].
func NewWithBaseURL(base *url.URL, opts ...Option) *Client {
	c := &Client{
		base:      cloneURL(base),
		http:      &http.Client{},
		userAgent: "n8n-cli",
		timeout:   DefaultTimeout,
		logger:    slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// BaseURL returns a copy of the API base URL, including the /api/v1 prefix.
func (c *Client) BaseURL() *url.URL { return cloneURL(c.base) }

// AuthType reports the configured mechanism, or the empty string when the
// client sends no credential.
func (c *Client) AuthType() AuthType {
	if c.auth == nil {
		return ""
	}
	return c.auth.Type()
}

// Request is one API call. Path is relative to the API base and must already be
// escaped; build it with [PathJoin].
type Request struct {
	Method string      // defaults to GET
	Path   string      // escaped path relative to the API base, e.g. "/workflows/1"
	Query  url.Values  // encoded in sorted order
	Body   any         // io.Reader sent as-is, anything else JSON-encoded, nil for none
	Header http.Header // extra headers; authentication headers are set afterwards
	// ContentType overrides the request content type. Defaults to
	// application/json for encoded bodies.
	ContentType string
	// Accept overrides the Accept header. Defaults to application/json.
	Accept string
}

// Response is the metadata of a successful call. The body is already decoded.
type Response struct {
	StatusCode int
	Header     http.Header
	RequestID  string
}

// Do sends r and decodes a JSON response body into out.
//
// out may be nil, in which case the body is discarded. A 204 or otherwise empty
// body leaves out untouched. Any non-2xx status returns an [*APIError] with the
// status, request ID and decoded API message preserved.
func (c *Client) Do(ctx context.Context, r Request, out any) (*Response, error) {
	if ctx == nil {
		return nil, errors.New("n8n: nil context")
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	method := r.Method
	if method == "" {
		method = http.MethodGet
	}

	endpoint, err := c.resolve(r.Path, r.Query)
	if err != nil {
		return nil, err
	}

	body, contentType, err := encodeBody(r.Body, r.ContentType)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("n8n: build request: %w", err)
	}
	if seeker, ok := body.(*bytes.Reader); ok {
		req.ContentLength = int64(seeker.Len())
		req.GetBody = func() (io.ReadCloser, error) {
			clone := *seeker
			_, err := clone.Seek(0, io.SeekStart)
			return io.NopCloser(&clone), err
		}
	}

	for name, values := range r.Header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	accept := r.Accept
	if accept == "" {
		accept = contentTypeJSON
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	// Last, so no caller-supplied header can displace the credential.
	if c.auth != nil {
		c.auth.Apply(req)
	}

	start := time.Now()
	c.logger.DebugContext(ctx, "n8n request",
		"method", method,
		"url", endpoint.Redacted(),
		"auth", c.AuthType(),
	)

	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.DebugContext(ctx, "n8n request failed",
			"method", method,
			"url", endpoint.Redacted(),
			"duration", time.Since(start),
			"error", err,
		)
		// A canceled or timed-out context surfaces as context.Canceled or
		// context.DeadlineExceeded through the url.Error wrapper.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("n8n: %s %s: %w", method, r.Path, ctxErr)
		}
		return nil, fmt.Errorf("n8n: %s %s: %w", method, r.Path, err)
	}
	defer drainAndClose(resp.Body)

	result := &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		RequestID:  requestID(resp.Header),
	}
	c.logger.DebugContext(ctx, "n8n response",
		"method", method,
		"url", endpoint.Redacted(),
		"status", resp.StatusCode,
		"requestID", result.RequestID,
		"duration", time.Since(start),
	)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, truncated, err := readLimited(resp.Body, maxErrorBody)
		if err != nil {
			return result, fmt.Errorf("n8n: %s %s: read error response: %w", method, r.Path, err)
		}
		return result, newAPIError(resp, method, r.Path, body, truncated)
	}

	if out == nil {
		return result, nil
	}
	if err := decodeBody(resp, out); err != nil {
		return result, fmt.Errorf("n8n: %s %s: %w", method, r.Path, err)
	}
	return result, nil
}

// resolve joins a request path and query onto the API base URL.
func (c *Client) resolve(path string, query url.Values) (*url.URL, error) {
	endpoint := cloneURL(c.base)
	joined := strings.TrimSuffix(endpoint.EscapedPath(), "/")
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		joined += path
	}
	if err := setEscapedPath(endpoint, joined); err != nil {
		return nil, fmt.Errorf("n8n: invalid request path %q: %w", path, err)
	}
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	return endpoint, nil
}

// encodeBody turns a request body into a reader and its content type.
func encodeBody(body any, contentType string) (io.Reader, string, error) {
	switch v := body.(type) {
	case nil:
		return nil, "", nil
	case io.Reader:
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return v, contentType, nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, "", fmt.Errorf("n8n: encode request body: %w", err)
		}
		if contentType == "" {
			contentType = contentTypeJSON
		}
		return bytes.NewReader(encoded), contentType, nil
	}
}

// decodeBody decodes a successful response into out, tolerating the empty
// bodies that 204 and some 200 replacements return.
func decodeBody(resp *http.Response, out any) error {
	if resp.StatusCode == http.StatusNoContent || resp.ContentLength == 0 {
		return nil
	}

	// One byte of lookahead distinguishes an empty body from a JSON document
	// when the server sends no Content-Length.
	br := bufferedPeek(resp.Body)
	if br.empty {
		return nil
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil {
			return fmt.Errorf("unreadable Content-Type %q", ct)
		}
		if mediaType != contentTypeJSON && !strings.HasSuffix(mediaType, "+json") {
			preview, _, _ := readLimited(br.reader, 512)
			return fmt.Errorf("unexpected content type %q: %s", mediaType, singleLine(string(preview)))
		}
	}

	if err := json.NewDecoder(br.reader).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

type peeked struct {
	reader io.Reader
	empty  bool
}

func bufferedPeek(body io.Reader) peeked {
	var first [1]byte
	n, err := io.ReadFull(body, first[:])
	if n == 0 {
		// io.EOF here means an empty body; any other error resurfaces on decode.
		return peeked{reader: body, empty: err == io.EOF || err == io.ErrUnexpectedEOF}
	}
	return peeked{reader: io.MultiReader(bytes.NewReader(first[:n]), body)}
}

// readLimited reads at most limit bytes and reports whether more remained.
func readLimited(r io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

// drainAndClose lets keep-alive reuse the connection without reading an
// unbounded amount of an abandoned body.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxErrorBody))
	_ = body.Close()
}

func cloneURL(u *url.URL) *url.URL {
	clone := *u
	return &clone
}

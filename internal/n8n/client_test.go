package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// capture records what the test server received.
type capture struct {
	mu     sync.Mutex
	method string
	path   string // escaped path, as it arrived on the wire
	query  string
	header http.Header
	body   string
}

func (c *capture) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.method = r.Method
	c.path = r.URL.EscapedPath()
	c.query = r.URL.RawQuery
	c.header = r.Header.Clone()
	c.body = string(body)
}

// query builds url.Values from alternating key and value arguments.
func query(pairs ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		v.Set(pairs[i], pairs[i+1])
	}
	return v
}

// newTestClient starts a server running handler and returns a client pointed at
// it, plus the request capture.
func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.record(r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, opts...)
	if err != nil {
		t.Fatalf("New(%q): %v", srv.URL, err)
	}
	return client, got
}

func TestDoSendsRequest(t *testing.T) {
	auth, err := NewAPIKeyAuth(testSecret)
	if err != nil {
		t.Fatalf("NewAPIKeyAuth: %v", err)
	}

	client, got := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.Header().Set("X-Request-Id", "req-7")
		fmt.Fprint(w, `{"id":"42","name":"nightly"}`)
	}, WithAuth(auth), WithUserAgent("n8n-cli/1.2.3 (linux/amd64)"))

	var out struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	resp, err := client.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   PathJoin("workflows", "a/b", "activate"),
		Query:  query("limit", "10", "cursor", "abc", "active", "true"),
		Body:   map[string]string{"name": "nightly"},
		Header: http.Header{"X-Trace": {"t-1"}},
	}, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	// The workflow ID contains a slash: it must stay one escaped segment.
	if want := "/api/v1/workflows/a%2Fb/activate"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	// url.Values.Encode sorts keys, so the query is deterministic.
	if want := "active=true&cursor=abc&limit=10"; got.query != want {
		t.Errorf("query = %q, want %q", got.query, want)
	}
	if want := `{"name":"nightly"}`; got.body != want {
		t.Errorf("body = %q, want %q", got.body, want)
	}
	for header, want := range map[string]string{
		HeaderAPIKey:   testSecret,
		"Content-Type": contentTypeJSON,
		"Accept":       contentTypeJSON,
		"User-Agent":   "n8n-cli/1.2.3 (linux/amd64)",
		"X-Trace":      "t-1",
	} {
		if v := got.header.Get(header); v != want {
			t.Errorf("%s = %q, want %q", header, v, want)
		}
	}
	if out.ID != "42" || out.Name != "nightly" {
		t.Errorf("decoded = %+v, want id 42 and name nightly", out)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if resp.RequestID != "req-7" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-7")
	}
}

func TestDoDefaultsToGETWithoutBody(t *testing.T) {
	client, got := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		fmt.Fprint(w, `{}`)
	})

	if _, err := client.Do(context.Background(), Request{Path: "/discover"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.body != "" {
		t.Errorf("body = %q, want empty", got.body)
	}
	if ct := got.header.Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q, want unset", ct)
	}
	if got.query != "" {
		t.Errorf("query = %q, want empty", got.query)
	}
}

func TestDoWithoutAuthSendsNoCredential(t *testing.T) {
	client, got := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if _, err := client.Do(context.Background(), Request{Path: "/discover"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	for _, header := range []string{HeaderAPIKey, "Authorization", "Cookie"} {
		if v := got.header.Get(header); v != "" {
			t.Errorf("%s = %q, want unset", header, v)
		}
	}
	if client.AuthType() != "" {
		t.Errorf("AuthType() = %q, want empty", client.AuthType())
	}
}

// A caller-supplied header must not be able to displace the credential.
func TestDoAuthWinsOverCallerHeader(t *testing.T) {
	auth, _ := NewAPIKeyAuth(testSecret)
	client, got := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, WithAuth(auth))

	_, err := client.Do(context.Background(), Request{
		Path:   "/discover",
		Header: http.Header{HeaderAPIKey: {"attacker-key"}},
	}, nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if v := got.header.Get(HeaderAPIKey); v != testSecret {
		t.Errorf("%s = %q, want the configured credential", HeaderAPIKey, v)
	}
	if len(got.header.Values(HeaderAPIKey)) != 1 {
		t.Errorf("%s sent %d times, want 1", HeaderAPIKey, len(got.header.Values(HeaderAPIKey)))
	}
}

func TestDoEmptyResponses(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"204 no content", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}},
		{"200 with empty body", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentTypeJSON)
			w.WriteHeader(http.StatusOK)
		}},
		{"200 with no content type", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTestClient(t, tt.handler)
			out := map[string]any{"untouched": true}
			resp, err := client.Do(context.Background(), Request{Method: http.MethodDelete, Path: "/workflows/1"}, &out)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			if len(out) != 1 || out["untouched"] != true {
				t.Errorf("out = %v, want it untouched", out)
			}
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				t.Errorf("StatusCode = %d", resp.StatusCode)
			}
		})
	}
}

func TestDoAPIError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.Header().Set("X-Request-Id", "req-9")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"workflow not found"}`)
	})

	var out map[string]any
	resp, err := client.Do(context.Background(), Request{Path: PathJoin("workflows", "42")}, &out)
	if err == nil {
		t.Fatal("Do: want an error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T (%v), want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "workflow not found" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "workflow not found")
	}
	if apiErr.RequestID != "req-9" {
		t.Errorf("RequestID = %q, want %q", apiErr.RequestID, "req-9")
	}
	if apiErr.Method != http.MethodGet || apiErr.Path != "/workflows/42" {
		t.Errorf("Method, Path = %q, %q, want GET /workflows/42", apiErr.Method, apiErr.Path)
	}
	if !IsNotFound(err) {
		t.Error("IsNotFound() = false, want true")
	}
	// Response metadata is still useful when the call failed.
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("response = %+v, want the failed status", resp)
	}
	if out != nil {
		t.Errorf("out = %v, want it untouched on failure", out)
	}
}

func TestDoErrorDoesNotLeakCredential(t *testing.T) {
	auth, _ := NewBearerAuth(testSecret)
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"unauthorized"}`)
	}, WithAuth(auth))

	_, err := client.Do(context.Background(), Request{Path: "/discover"}, nil)
	if err == nil {
		t.Fatal("Do: want an error")
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Errorf("error leaks the credential: %v", err)
	}
	if !IsUnauthorized(err) {
		t.Error("IsUnauthorized() = false, want true")
	}
}

func TestDoMalformedJSON(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		fmt.Fprint(w, `{"id": `)
	})

	var out map[string]any
	_, err := client.Do(context.Background(), Request{Path: "/discover"}, &out)
	if err == nil {
		t.Fatal("Do: want an error")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error = %v, want a decode failure", err)
	}
}

func TestDoUnexpectedContentType(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body>login</body></html>")
	})

	var out map[string]any
	_, err := client.Do(context.Background(), Request{Path: "/discover"}, &out)
	if err == nil {
		t.Fatal("Do: want an error")
	}
	if !strings.Contains(err.Error(), "unexpected content type \"text/html\"") {
		t.Errorf("error = %v, want it to name the content type", err)
	}
	if !strings.Contains(err.Error(), "login") {
		t.Errorf("error = %v, want a body preview", err)
	}
}

func TestDoIgnoresBodyWhenOutIsNil(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>not json</html>")
	})

	if _, err := client.Do(context.Background(), Request{Path: "/discover"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestDoCanceledContext(t *testing.T) {
	release := make(chan struct{})
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := client.Do(ctx, Request{Path: "/discover"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestDoTimeout(t *testing.T) {
	release := make(chan struct{})
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusNoContent)
	}, WithTimeout(20*time.Millisecond))
	t.Cleanup(func() { close(release) })

	_, err := client.Do(context.Background(), Request{Path: "/discover"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestDoTransportFailure(t *testing.T) {
	client, err := New("http://127.0.0.1:1", WithTimeout(time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Do(context.Background(), Request{Path: "/discover"}, nil); err == nil {
		t.Fatal("Do: want a connection error")
	}
}

func TestDoRejectsUnencodableBody(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server was called, want the body rejected before sending")
	})

	_, err := client.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/workflows",
		Body:   map[string]any{"fn": func() {}},
	}, nil)
	if err == nil {
		t.Fatal("Do: want an encoding error")
	}
	if !strings.Contains(err.Error(), "encode request body") {
		t.Errorf("error = %v, want an encoding failure", err)
	}
}

func TestDoReaderBody(t *testing.T) {
	client, got := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	_, err := client.Do(context.Background(), Request{
		Method:      http.MethodPost,
		Path:        "/n8n-packages",
		Body:        strings.NewReader("raw-bytes"),
		ContentType: "application/gzip",
	}, nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got.body != "raw-bytes" {
		t.Errorf("body = %q, want %q", got.body, "raw-bytes")
	}
	if ct := got.header.Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type = %q, want application/gzip", ct)
	}
}

func TestDoRedirectReplaysBody(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		if r.URL.Path == "/api/v1/workflows" {
			http.Redirect(w, r, "/api/v1/workflows-moved", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	_, err := client.Do(context.Background(), Request{
		Method: http.MethodPost,
		Path:   "/workflows",
		Body:   map[string]string{"name": "x"},
	}, nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("redirected body = %q, want the original %q", bodies[1], bodies[0])
	}
}

func TestDoNilContext(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server was called, want the nil context rejected first")
	})

	//lint:ignore SA1012 the guard against a nil context is the behavior under test.
	if _, err := client.Do(nil, Request{Path: "/discover"}, nil); err == nil { //nolint:staticcheck
		t.Fatal("Do: want an error")
	}
}

func TestDebugLoggingRedactsAndDescribes(t *testing.T) {
	auth, _ := NewAPIKeyAuth(testSecret)
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.Header().Set("X-Request-Id", "req-5")
		fmt.Fprint(w, `{}`)
	}, WithAuth(auth), WithLogger(logger))

	var out map[string]any
	if _, err := client.Do(context.Background(), Request{Path: "/discover"}, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}

	log := logged.String()
	if strings.Contains(log, testSecret) {
		t.Errorf("debug log leaks the credential:\n%s", log)
	}
	for _, want := range []string{"n8n request", "n8n response", "/api/v1/discover", "status=200", "req-5"} {
		if !strings.Contains(log, want) {
			t.Errorf("debug log missing %q:\n%s", want, log)
		}
	}
}

func TestClientBaseURLIsACopy(t *testing.T) {
	client, err := New("https://n8n.example.com")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	base := client.BaseURL()
	if base.String() != "https://n8n.example.com/api/v1" {
		t.Errorf("BaseURL() = %q", base)
	}
	base.Path = "/tampered"
	if client.BaseURL().String() != "https://n8n.example.com/api/v1" {
		t.Error("BaseURL() exposes mutable client state")
	}
}

func TestNewRejectsBadURL(t *testing.T) {
	if _, err := New("ftp://n8n.example.com"); err == nil {
		t.Fatal("New: want an error")
	}
}

// The base path is applied once, whatever shape the request path arrives in.
func TestDoResolvesPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"leading slash", "/workflows", "/api/v1/workflows"},
		{"no leading slash", "workflows", "/api/v1/workflows"},
		{"empty", "", "/api/v1"},
		{"escaped segment", PathJoin("tags", "a b"), "/api/v1/tags/a%20b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, got := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			if _, err := client.Do(context.Background(), Request{Path: tt.path}, nil); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if got.path != tt.want {
				t.Errorf("path = %q, want %q", got.path, tt.want)
			}
		})
	}
}

func TestDoDecodesIntoRawMessage(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		fmt.Fprint(w, `{"unknown":"field"}`)
	})

	var out json.RawMessage
	if _, err := client.Do(context.Background(), Request{Path: "/discover"}, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(out) != `{"unknown":"field"}` {
		t.Errorf("out = %s, want the raw document", out)
	}
}

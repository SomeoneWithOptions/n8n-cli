package n8n

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// BasePath is the API prefix appended to a normalized instance URL exactly once.
const BasePath = "/api/v1"

// NormalizeBaseURL turns a user-supplied instance URL into the API base URL.
//
// It accepts a bare host ("n8n.example.com"), defaults the scheme to https,
// lowercases scheme and host, drops trailing slashes, and appends [BasePath]
// unless the URL already ends with it. Credentials embedded in the URL are
// rejected: they would be sent on every request and leak through logs, proxies
// and referrers.
func NormalizeBaseURL(raw string) (*url.URL, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("instance URL is empty")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse instance URL: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported instance URL scheme %q: use http or https", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("instance URL must not contain credentials")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("instance URL %q has no host", raw)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("instance URL %q has no host", raw)
	}
	if u.RawQuery != "" || u.ForceQuery {
		return nil, errors.New("instance URL must not contain a query string")
	}
	if u.Fragment != "" {
		return nil, errors.New("instance URL must not contain a fragment")
	}

	base := &url.URL{
		Scheme: strings.ToLower(u.Scheme),
		Host:   strings.ToLower(u.Host),
	}

	path := strings.TrimRight(u.EscapedPath(), "/")
	if !strings.HasSuffix(path, BasePath) {
		path += BasePath
	}
	if err := setEscapedPath(base, path); err != nil {
		return nil, fmt.Errorf("instance URL path: %w", err)
	}
	return base, nil
}

// PathJoin builds a request path from segments, escaping each one exactly once.
// Use it for every dynamic value: an ID containing "/" must stay one segment.
func PathJoin(segments ...string) string {
	escaped := make([]string, 0, len(segments))
	for _, s := range segments {
		if s == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(s))
	}
	return "/" + strings.Join(escaped, "/")
}

// setEscapedPath stores an already-escaped path on u, keeping Path and RawPath
// consistent so url.URL.String does not re-escape reserved characters.
func setEscapedPath(u *url.URL, escaped string) error {
	unescaped, err := url.PathUnescape(escaped)
	if err != nil {
		return err
	}
	u.Path = unescaped
	u.RawPath = escaped
	return nil
}

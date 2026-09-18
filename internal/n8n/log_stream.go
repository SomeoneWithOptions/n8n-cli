package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	LogStreamEventTypesPath   = "/settings/log-streaming/event-types"
	LogStreamDestinationsPath = "/settings/log-streaming/destinations"
)

// LogStreamEventTypes is the non-paginated set of streamable event names.
type LogStreamEventTypes struct {
	Data []string `json:"data"`
}

// LogStreamDestination models all three destination variants. Connection URLs,
// headers, query parameters, certificates and extensible HTTP options are secret
// material: ordinary JSON and formatting never reveal them. Unknown response
// fields are intentionally discarded, since they may contain new credentials.
type LogStreamDestination struct {
	ID                     string                   `json:"id,omitempty"`
	Type                   string                   `json:"type"`
	Label                  string                   `json:"label,omitempty"`
	Enabled                *bool                    `json:"enabled,omitempty"`
	SubscribedEvents       []string                 `json:"subscribedEvents,omitempty"`
	AnonymizeAuditMessages *bool                    `json:"anonymizeAuditMessages,omitempty"`
	CircuitBreaker         *LogStreamCircuitBreaker `json:"circuitBreaker,omitempty"`
	URL                    Secret                   `json:"url,omitempty"`
	Method                 string                   `json:"method,omitempty"`
	SendHeaders            *bool                    `json:"sendHeaders,omitempty"`
	SpecifyHeaders         string                   `json:"specifyHeaders,omitempty"`
	HeaderParameters       *CredentialData          `json:"headerParameters,omitempty"`
	JSONHeaders            Secret                   `json:"jsonHeaders,omitempty"`
	SendQuery              *bool                    `json:"sendQuery,omitempty"`
	SpecifyQuery           string                   `json:"specifyQuery,omitempty"`
	QueryParameters        *CredentialData          `json:"queryParameters,omitempty"`
	JSONQuery              Secret                   `json:"jsonQuery,omitempty"`
	Options                *CredentialData          `json:"options,omitempty"`
	Host                   string                   `json:"host,omitempty"`
	Port                   *int                     `json:"port,omitempty"`
	Protocol               string                   `json:"protocol,omitempty"`
	Facility               *int                     `json:"facility,omitempty"`
	AppName                string                   `json:"app_name,omitempty"`
	TLSCa                  Secret                   `json:"tlsCa,omitempty"`
	DSN                    Secret                   `json:"dsn,omitempty"`
}

type LogStreamCircuitBreaker struct {
	MaxFailures   *int `json:"maxFailures,omitempty"`
	FailureWindow *int `json:"failureWindow,omitempty"`
}

type LogStreamDestinations struct {
	Data []LogStreamDestination `json:"data"`
}
type LogStreamTestResult struct {
	Success bool `json:"success"`
}

// LogStreamDestinationRequest retains additional properties permitted by the
// variant schemas, but exposes them only to the private transport boundary.
// Construct with JSON unmarshaling. A read-only id is stripped for GET-edit-PUT.
type LogStreamDestinationRequest struct{ raw json.RawMessage }

func (r LogStreamDestinationRequest) String() string               { return Redacted }
func (r LogStreamDestinationRequest) GoString() string             { return Redacted }
func (r LogStreamDestinationRequest) LogValue() slog.Value         { return slog.StringValue(Redacted) }
func (r LogStreamDestinationRequest) MarshalJSON() ([]byte, error) { return json.Marshal(Redacted) }
func (r *LogStreamDestinationRequest) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' || !json.Valid(raw) {
		return fmt.Errorf("destination must be a JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("destination must be a JSON object")
	}
	delete(fields, "id")
	encoded, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("invalid destination JSON")
	}
	candidate := LogStreamDestinationRequest{raw: encoded}
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}

// Validate checks the documented required variant fields, enums and ranges.
// Extra fields are forwarded, not silently dropped: the schema allows them.
func (r LogStreamDestinationRequest) Validate() error {
	if len(r.raw) == 0 {
		return fmt.Errorf("destination JSON is required")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(r.raw, &fields); err != nil {
		return fmt.Errorf("invalid destination JSON")
	}
	// Never replace real credentials with the redacted output of a read command.
	var values any
	if err := json.Unmarshal(r.raw, &values); err != nil {
		return fmt.Errorf("invalid destination JSON")
	}
	if containsLogStreamRedaction(values) {
		return fmt.Errorf("destination contains [REDACTED] placeholders: restore original secrets from a protected source before submitting")
	}
	var d LogStreamDestination
	if err := json.Unmarshal(r.raw, &d); err != nil {
		return fmt.Errorf("destination field has an invalid JSON type or shape (details redacted)")
	}
	for _, name := range []string{"type", "label", "enabled", "subscribedEvents", "anonymizeAuditMessages", "circuitBreaker", "url", "method", "sendHeaders", "specifyHeaders", "headerParameters", "jsonHeaders", "sendQuery", "specifyQuery", "queryParameters", "jsonQuery", "options", "host", "port", "protocol", "facility", "app_name", "tlsCa", "dsn"} {
		if bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return fmt.Errorf("destination %s cannot be null; omit unused optional fields", name)
		}
	}
	switch d.Type {
	case "webhook":
		if !logStreamURI(d.URL.Reveal()) {
			return fmt.Errorf("webhook destination requires url with an absolute URI")
		}
		if err := validateLogStreamParameters(fields, "headerParameters"); err != nil {
			return err
		}
		if err := validateLogStreamParameters(fields, "queryParameters"); err != nil {
			return err
		}
		// Decode known option types without restricting additional properties.
		var options struct {
			Timeout                *int    `json:"timeout"`
			AllowUnauthorizedCerts *bool   `json:"allowUnauthorizedCerts"`
			QueryParameterArrays   *string `json:"queryParameterArrays"`
			Redirect               *struct {
				Redirect *struct {
					FollowRedirects *bool `json:"followRedirects"`
					MaxRedirects    *int  `json:"maxRedirects"`
				} `json:"redirect"`
			} `json:"redirect"`
			Proxy *struct {
				Proxy *struct {
					Protocol *string `json:"protocol"`
					Host     string  `json:"host"`
					Port     *int    `json:"port"`
				} `json:"proxy"`
			} `json:"proxy"`
			Socket *struct {
				KeepAlive      *bool `json:"keepAlive"`
				MaxSockets     *int  `json:"maxSockets"`
				MaxFreeSockets *int  `json:"maxFreeSockets"`
			} `json:"socket"`
		}
		if raw, ok := fields["options"]; ok {
			if err := json.Unmarshal(raw, &options); err != nil {
				return fmt.Errorf("webhook options have an invalid JSON type (details redacted)")
			}
			if options.QueryParameterArrays != nil {
				switch *options.QueryParameterArrays {
				case "indices", "brackets", "repeat":
				default:
					return fmt.Errorf("options.queryParameterArrays must be indices, brackets, or repeat")
				}
			}
			if options.Proxy != nil && options.Proxy.Proxy != nil && options.Proxy.Proxy.Protocol != nil {
				switch *options.Proxy.Proxy.Protocol {
				case "https", "http":
				default:
					return fmt.Errorf("options.proxy.proxy.protocol must be https or http")
				}
			}
		}
	case "syslog":
		if strings.TrimSpace(d.Host) == "" {
			return fmt.Errorf("syslog destination requires host")
		}
		if _, present := fields["protocol"]; present {
			switch d.Protocol {
			case "udp", "tcp", "tls":
			default:
				return fmt.Errorf("syslog protocol must be udp, tcp, or tls")
			}
		}
		if d.Facility != nil && (*d.Facility < 0 || *d.Facility > 23) {
			return fmt.Errorf("syslog facility must be between 0 and 23")
		}
	case "sentry":
		if !logStreamURI(d.DSN.Reveal()) {
			return fmt.Errorf("sentry destination requires dsn with an absolute URI")
		}
	default:
		return fmt.Errorf("destination type must be webhook, syslog, or sentry")
	}
	return nil
}

func logStreamURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && (parsed.Host != "" || parsed.Opaque != "")
}

func containsLogStreamRedaction(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, Redacted)
	case []any:
		return slices.ContainsFunc(v, containsLogStreamRedaction)
	case map[string]any:
		for _, item := range v {
			if containsLogStreamRedaction(item) {
				return true
			}
		}
	}
	return false
}

func validateLogStreamParameters(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	var list struct {
		Parameters []struct {
			Name  *string         `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("%s must contain a parameters array with name/value pairs", name)
	}
	for _, p := range list.Parameters {
		if p.Name == nil || len(p.Value) == 0 {
			return fmt.Errorf("%s parameters require name and value (null values are accepted)", name)
		}
	}
	return nil
}

func (c *Client) GetLogStreamEventTypes(ctx context.Context) (*LogStreamEventTypes, error) {
	result := LogStreamEventTypes{Data: []string{}}
	if _, err := c.Do(ctx, Request{Path: LogStreamEventTypesPath}, &result); err != nil {
		return nil, redactLogStreamError(err)
	}
	if result.Data == nil {
		result.Data = []string{}
	}
	return &result, nil
}
func (c *Client) ListLogStreamDestinations(ctx context.Context) (*LogStreamDestinations, error) {
	result := LogStreamDestinations{Data: []LogStreamDestination{}}
	if _, err := c.Do(ctx, Request{Path: LogStreamDestinationsPath}, &result); err != nil {
		return nil, redactLogStreamError(err)
	}
	if result.Data == nil {
		result.Data = []LogStreamDestination{}
	}
	return &result, nil
}
func (c *Client) GetLogStreamDestination(ctx context.Context, id string) (*LogStreamDestination, error) {
	return c.logStreamDestination(ctx, http.MethodGet, id, nil)
}
func (c *Client) CreateLogStreamDestination(ctx context.Context, request LogStreamDestinationRequest) (*LogStreamDestination, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return c.logStreamDestination(ctx, http.MethodPost, "", request.raw)
}
func (c *Client) UpdateLogStreamDestination(ctx context.Context, id string, request LogStreamDestinationRequest) (*LogStreamDestination, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return c.logStreamDestination(ctx, http.MethodPut, id, request.raw)
}
func (c *Client) DeleteLogStreamDestination(ctx context.Context, id string) (*LogStreamDestination, error) {
	return c.logStreamDestination(ctx, http.MethodDelete, id, nil)
}
func (c *Client) logStreamDestination(ctx context.Context, method, id string, body any) (*LogStreamDestination, error) {
	path := LogStreamDestinationsPath
	if method != http.MethodPost {
		if err := validateLogStreamID(id); err != nil {
			return nil, err
		}
		path += PathJoin(id)
	}
	var result LogStreamDestination
	if _, err := c.Do(ctx, Request{Method: method, Path: path, Body: body}, &result); err != nil {
		return nil, redactLogStreamError(err)
	}
	return &result, nil
}
func (c *Client) TestLogStreamDestination(ctx context.Context, id string) (*LogStreamTestResult, error) {
	if err := validateLogStreamID(id); err != nil {
		return nil, err
	}
	var result LogStreamTestResult
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: LogStreamDestinationsPath + PathJoin(id, "test")}, &result); err != nil {
		return nil, redactLogStreamError(err)
	}
	return &result, nil
}
func validateLogStreamID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("destination ID is required")
	}
	return nil
}

// Redact all server-controlled error details: even read/test/delete errors may
// echo stored destination credentials. Preserve status, method and request ID.
func redactLogStreamError(err error) error {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		clone := *apiErr
		clone.Code, clone.Hint, clone.Body = "", "", ""
		clone.Message = "log streaming request failed; response details redacted"
		clone.Truncated = false
		return &clone
	}
	var syntax *json.SyntaxError
	var fieldType *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &fieldType) {
		return fmt.Errorf("invalid log streaming response JSON (details redacted)")
	}
	return err
}

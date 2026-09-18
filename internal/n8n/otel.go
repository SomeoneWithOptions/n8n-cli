package n8n

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
)

const (
	// OtelSettingsPath is the instance-wide OpenTelemetry settings endpoint.
	OtelSettingsPath = "/settings/otel"
	// OtelTestTracePath sends one test span without changing stored settings.
	OtelTestTracePath = "/settings/otel/test-trace"
)

// OtelExporterProtocol is the OTLP wire protocol used by the exporter.
type OtelExporterProtocol string

const (
	OtelProtocolHTTPProtobuf OtelExporterProtocol = "http/protobuf"
	OtelProtocolGRPC         OtelExporterProtocol = "grpc"
)

// OtelSettings is the effective instance-wide OpenTelemetry configuration.
type OtelSettings struct {
	Enabled                      bool                 `json:"enabled"`
	ExporterProtocol             OtelExporterProtocol `json:"exporterProtocol"`
	ExporterEndpoint             string               `json:"exporterEndpoint"`
	ExporterTracingPath          string               `json:"exporterTracingPath"`
	ExporterServiceName          string               `json:"exporterServiceName"`
	ExporterHeaders              string               `json:"exporterHeaders"`
	TracesSampleRate             float64              `json:"tracesSampleRate"`
	StartupConnectivityTimeoutMs int                  `json:"startupConnectivityTimeoutMs"`
	IncludeNodeSpans             bool                 `json:"includeNodeSpans"`
	InjectOutbound               bool                 `json:"injectOutbound"`
	ProductionExecutionsOnly     bool                 `json:"productionExecutionsOnly"`
}

func (s *OtelSettings) applyDefaults() {
	if s.ExporterProtocol == "" {
		s.ExporterProtocol = OtelProtocolHTTPProtobuf
	}
}

// UpdateOtelSettingsRequest is a full replacement document. Pointers preserve
// omitted required fields separately from valid false, zero, and empty values.
// ExporterProtocol is optional; omission selects http/protobuf on the server.
type UpdateOtelSettingsRequest struct {
	Enabled                      *bool                `json:"enabled"`
	ExporterProtocol             OtelExporterProtocol `json:"exporterProtocol,omitempty"`
	ExporterEndpoint             *string              `json:"exporterEndpoint"`
	ExporterTracingPath          *string              `json:"exporterTracingPath"`
	ExporterServiceName          *string              `json:"exporterServiceName"`
	ExporterHeaders              *string              `json:"exporterHeaders"`
	TracesSampleRate             *float64             `json:"tracesSampleRate"`
	StartupConnectivityTimeoutMs *int                 `json:"startupConnectivityTimeoutMs"`
	IncludeNodeSpans             *bool                `json:"includeNodeSpans"`
	InjectOutbound               *bool                `json:"injectOutbound"`
	ProductionExecutionsOnly     *bool                `json:"productionExecutionsOnly"`
}

// Validate enforces every required replacement field and documented range
// before transport.
func (r UpdateOtelSettingsRequest) Validate() error {
	if r.Enabled == nil {
		return fmt.Errorf("enabled is required and must be true or false")
	}
	if err := validateOtelProtocol(r.ExporterProtocol); err != nil {
		return err
	}
	if err := validateOtelConnection(r.ExporterEndpoint, r.ExporterTracingPath, r.ExporterServiceName, r.ExporterHeaders, r.StartupConnectivityTimeoutMs); err != nil {
		return err
	}
	if r.TracesSampleRate == nil {
		return fmt.Errorf("tracesSampleRate is required")
	}
	if math.IsNaN(*r.TracesSampleRate) || math.IsInf(*r.TracesSampleRate, 0) || *r.TracesSampleRate < 0 || *r.TracesSampleRate > 1 {
		return fmt.Errorf("tracesSampleRate must be a finite number between 0 and 1")
	}
	if r.IncludeNodeSpans == nil {
		return fmt.Errorf("includeNodeSpans is required and must be true or false")
	}
	if r.InjectOutbound == nil {
		return fmt.Errorf("injectOutbound is required and must be true or false")
	}
	if r.ProductionExecutionsOnly == nil {
		return fmt.Errorf("productionExecutionsOnly is required and must be true or false")
	}
	return nil
}

// OtelTestTraceRequest contains collector details for one test span. It does
// not modify stored settings. ExporterProtocol defaults to http/protobuf.
type OtelTestTraceRequest struct {
	ExporterProtocol             OtelExporterProtocol `json:"exporterProtocol,omitempty"`
	ExporterEndpoint             *string              `json:"exporterEndpoint"`
	ExporterTracingPath          *string              `json:"exporterTracingPath"`
	ExporterServiceName          *string              `json:"exporterServiceName"`
	ExporterHeaders              *string              `json:"exporterHeaders"`
	StartupConnectivityTimeoutMs *int                 `json:"startupConnectivityTimeoutMs"`
}

// Validate checks test connection details before transport.
func (r OtelTestTraceRequest) Validate() error {
	if err := validateOtelProtocol(r.ExporterProtocol); err != nil {
		return err
	}
	return validateOtelConnection(r.ExporterEndpoint, r.ExporterTracingPath, r.ExporterServiceName, r.ExporterHeaders, r.StartupConnectivityTimeoutMs)
}

// OtelTestTraceResult is the collector's response to one test span. A false
// result is still a successful API operation and can include an error message.
type OtelTestTraceResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func validateOtelProtocol(protocol OtelExporterProtocol) error {
	switch protocol {
	case "", OtelProtocolHTTPProtobuf, OtelProtocolGRPC:
		return nil
	default:
		return fmt.Errorf("exporterProtocol must be http/protobuf or grpc (omit it to default to http/protobuf)")
	}
}

func validateOtelConnection(endpoint, tracingPath, serviceName, headers *string, timeout *int) error {
	if endpoint == nil {
		return fmt.Errorf("exporterEndpoint is required")
	}
	parsed, err := url.Parse(*endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("exporterEndpoint must be an http:// or https:// URL with a host")
	}
	if tracingPath == nil {
		return fmt.Errorf("exporterTracingPath is required (use an empty string when unused by grpc)")
	}
	if serviceName == nil || *serviceName == "" {
		return fmt.Errorf("exporterServiceName is required and must not be empty")
	}
	if headers == nil {
		return fmt.Errorf("exporterHeaders is required (use an empty string when unused)")
	}
	if timeout == nil {
		return fmt.Errorf("startupConnectivityTimeoutMs is required")
	}
	if *timeout < 0 {
		return fmt.Errorf("startupConnectivityTimeoutMs must be at least 0")
	}
	return nil
}

// GetOtelSettings returns the effective instance-wide OpenTelemetry settings.
func (c *Client) GetOtelSettings(ctx context.Context) (*OtelSettings, error) {
	var settings OtelSettings
	if _, err := c.Do(ctx, Request{Path: OtelSettingsPath}, &settings); err != nil {
		return nil, err
	}
	settings.applyDefaults()
	return &settings, nil
}

// UpdateOtelSettings fully replaces the OpenTelemetry configuration. The
// server applies a successful update to the running instance immediately.
func (c *Client) UpdateOtelSettings(ctx context.Context, request UpdateOtelSettingsRequest) (*OtelSettings, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var settings OtelSettings
	if _, err := c.Do(ctx, Request{Method: http.MethodPut, Path: OtelSettingsPath, Body: request}, &settings); err != nil {
		return nil, redactOtelAPIError(err)
	}
	settings.applyDefaults()
	return &settings, nil
}

// TestOtelTrace sends one span with supplied collector details without
// changing the stored OpenTelemetry configuration.
func (c *Client) TestOtelTrace(ctx context.Context, request OtelTestTraceRequest) (*OtelTestTraceResult, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result OtelTestTraceResult
	if _, err := c.Do(ctx, Request{Method: http.MethodPost, Path: OtelTestTracePath, Body: request}, &result); err != nil {
		return nil, redactOtelAPIError(err)
	}
	result.Error = redactOtelCollectorError(result.Error, request.ExporterHeaders)
	return &result, nil
}

// redactOtelCollectorError removes supplied header material if an exporter or
// collector includes it in an otherwise successful test result.
func redactOtelCollectorError(message string, headers *string) string {
	if headers == nil || *headers == "" || message == "" {
		return message
	}
	message = strings.ReplaceAll(message, *headers, "[redacted exporter headers]")
	for pair := range strings.SplitSeq(*headers, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		message = strings.ReplaceAll(message, pair, "[redacted exporter header]")
		if _, value, ok := strings.Cut(pair, "="); ok {
			value = strings.TrimSpace(value)
			if len(value) >= 4 {
				message = strings.ReplaceAll(message, value, "[redacted]")
			}
		}
	}
	return message
}

// redactOtelAPIError prevents collector authorization headers from being
// echoed by server-controlled validation errors while preserving diagnostics.
func redactOtelAPIError(err error) error {
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		return err
	}
	clone := *apiErr
	clone.Code = ""
	clone.Message = "OpenTelemetry request failed; response details redacted"
	clone.Hint = ""
	clone.Body = ""
	clone.Truncated = false
	return &clone
}

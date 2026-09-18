package n8n

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
)

const otelSettingsResponse = `{
  "enabled":true,
  "exporterProtocol":"grpc",
  "exporterEndpoint":"https://collector.example.com:4317",
  "exporterTracingPath":"/v1/traces",
  "exporterServiceName":"n8n-production",
  "exporterHeaders":"authorization=Bearer collector-secret",
  "tracesSampleRate":0.25,
  "startupConnectivityTimeoutMs":2500,
  "includeNodeSpans":false,
  "injectOutbound":true,
  "productionExecutionsOnly":true
}`

func ptr[T any](value T) *T { return &value }

func validOtelUpdateRequest() UpdateOtelSettingsRequest {
	return UpdateOtelSettingsRequest{
		Enabled:                      ptr(false),
		ExporterProtocol:             OtelProtocolGRPC,
		ExporterEndpoint:             ptr("https://collector.example.com:4317"),
		ExporterTracingPath:          ptr(""),
		ExporterServiceName:          ptr("n8n-production"),
		ExporterHeaders:              ptr("authorization=Bearer collector-secret"),
		TracesSampleRate:             ptr(0.0),
		StartupConnectivityTimeoutMs: ptr(0),
		IncludeNodeSpans:             ptr(false),
		InjectOutbound:               ptr(false),
		ProductionExecutionsOnly:     ptr(false),
	}
}

func validOtelTestRequest() OtelTestTraceRequest {
	return OtelTestTraceRequest{
		ExporterEndpoint:             ptr("http://collector.example.com:4318"),
		ExporterTracingPath:          ptr("/v1/traces"),
		ExporterServiceName:          ptr("n8n-test"),
		ExporterHeaders:              ptr(""),
		StartupConnectivityTimeoutMs: ptr(2000),
	}
}

func TestGetOtelSettings(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, otelSettingsResponse))
	settings, err := server.client(t).GetOtelSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOtelSettings: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+OtelSettingsPath || req.URL.RawQuery != "" || body != "" {
		t.Errorf("request = %s %s?%s body %q", req.Method, req.URL.EscapedPath(), req.URL.RawQuery, body)
	}
	if got := req.Header.Get(HeaderAPIKey); got != "community-secret" {
		t.Errorf("%s = %q, want API key", HeaderAPIKey, got)
	}
	if !settings.Enabled || settings.ExporterProtocol != OtelProtocolGRPC || settings.ExporterEndpoint != "https://collector.example.com:4317" ||
		settings.ExporterServiceName != "n8n-production" || settings.ExporterHeaders != "authorization=Bearer collector-secret" ||
		settings.TracesSampleRate != 0.25 || settings.StartupConnectivityTimeoutMs != 2500 || settings.IncludeNodeSpans ||
		!settings.InjectOutbound || !settings.ProductionExecutionsOnly {
		t.Errorf("settings = %+v", settings)
	}
}

func TestGetOtelSettingsAppliesDefaultProtocol(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"enabled":false}`))
	settings, err := server.client(t).GetOtelSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOtelSettings: %v", err)
	}
	if settings.ExporterProtocol != OtelProtocolHTTPProtobuf {
		t.Errorf("protocol = %q, want server default %q", settings.ExporterProtocol, OtelProtocolHTTPProtobuf)
	}
}

func TestUpdateOtelSettingsFullReplacement(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, otelSettingsResponse))
	request := validOtelUpdateRequest()
	settings, err := server.client(t).UpdateOtelSettings(context.Background(), request)
	if err != nil {
		t.Fatalf("UpdateOtelSettings: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodPut || req.URL.EscapedPath() != BasePath+OtelSettingsPath || req.URL.RawQuery != "" {
		t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	if sent["enabled"] != false || sent["exporterProtocol"] != "grpc" || sent["exporterTracingPath"] != "" ||
		sent["tracesSampleRate"] != float64(0) || sent["startupConnectivityTimeoutMs"] != float64(0) ||
		sent["includeNodeSpans"] != false || sent["injectOutbound"] != false || sent["productionExecutionsOnly"] != false {
		t.Errorf("false and zero replacement values not preserved: %#v", sent)
	}
	if settings.ExporterProtocol != OtelProtocolGRPC || settings.TracesSampleRate != 0.25 {
		t.Errorf("decoded settings = %+v", settings)
	}
}

func TestUpdateOtelSettingsOmitsProtocolForServerDefault(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"enabled":false}`))
	request := validOtelUpdateRequest()
	request.ExporterProtocol = ""
	settings, err := server.client(t).UpdateOtelSettings(context.Background(), request)
	if err != nil {
		t.Fatalf("UpdateOtelSettings: %v", err)
	}
	_, body := server.last(t)
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, ok := sent["exporterProtocol"]; ok {
		t.Errorf("optional exporterProtocol sent: %#v", sent)
	}
	if settings.ExporterProtocol != OtelProtocolHTTPProtobuf {
		t.Errorf("response protocol = %q, want default", settings.ExporterProtocol)
	}
}

func TestOtelTraceResult(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response string
		success  bool
		message  string
	}{
		{name: "accepted", response: `{"success":true}`, success: true},
		{name: "collector rejected", response: `{"success":false,"error":"401 Unauthorized"}`, message: "401 Unauthorized"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, tt.response))
			result, err := server.client(t).TestOtelTrace(context.Background(), validOtelTestRequest())
			if err != nil {
				t.Fatalf("TestOtelTrace: %v", err)
			}
			req, body := server.last(t)
			if req.Method != http.MethodPost || req.URL.EscapedPath() != BasePath+OtelTestTracePath || req.URL.RawQuery != "" {
				t.Errorf("request = %s %s?%s", req.Method, req.URL.EscapedPath(), req.URL.RawQuery)
			}
			var sent map[string]any
			if err := json.Unmarshal([]byte(body), &sent); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if _, ok := sent["exporterProtocol"]; ok {
				t.Error("omitted protocol was sent instead of using server default")
			}
			if result.Success != tt.success || result.Error != tt.message {
				t.Errorf("result = %+v", result)
			}
		})
	}
}

func TestOtelTraceRedactsSuppliedHeadersFromResult(t *testing.T) {
	const secret = "collector-test-secret"
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"success":false,"error":"collector echoed Bearer collector-test-secret"}`))
	request := validOtelTestRequest()
	request.ExporterHeaders = ptr("authorization=Bearer " + secret)
	result, err := server.client(t).TestOtelTrace(context.Background(), request)
	if err != nil {
		t.Fatalf("TestOtelTrace: %v", err)
	}
	if strings.Contains(result.Error, secret) || !strings.Contains(result.Error, "[redacted]") {
		t.Errorf("result error was not safely redacted: %q", result.Error)
	}
}

func TestUpdateOtelSettingsValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*UpdateOtelSettingsRequest)
		want   string
	}{
		{name: "missing enabled", mutate: func(r *UpdateOtelSettingsRequest) { r.Enabled = nil }, want: "enabled"},
		{name: "invalid protocol", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterProtocol = "json" }, want: "http/protobuf or grpc"},
		{name: "missing endpoint", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterEndpoint = nil }, want: "exporterEndpoint"},
		{name: "invalid endpoint scheme", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterEndpoint = ptr("ftp://collector.example.com") }, want: "http:// or https://"},
		{name: "missing endpoint host", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterEndpoint = ptr("https:///traces") }, want: "with a host"},
		{name: "missing tracing path", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterTracingPath = nil }, want: "exporterTracingPath"},
		{name: "empty service", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterServiceName = ptr("") }, want: "exporterServiceName"},
		{name: "missing headers", mutate: func(r *UpdateOtelSettingsRequest) { r.ExporterHeaders = nil }, want: "exporterHeaders"},
		{name: "missing sample rate", mutate: func(r *UpdateOtelSettingsRequest) { r.TracesSampleRate = nil }, want: "tracesSampleRate"},
		{name: "sample below range", mutate: func(r *UpdateOtelSettingsRequest) { r.TracesSampleRate = ptr(-0.01) }, want: "between 0 and 1"},
		{name: "sample above range", mutate: func(r *UpdateOtelSettingsRequest) { r.TracesSampleRate = ptr(1.01) }, want: "between 0 and 1"},
		{name: "sample not finite", mutate: func(r *UpdateOtelSettingsRequest) { r.TracesSampleRate = ptr(math.NaN()) }, want: "finite number"},
		{name: "missing timeout", mutate: func(r *UpdateOtelSettingsRequest) { r.StartupConnectivityTimeoutMs = nil }, want: "startupConnectivityTimeoutMs"},
		{name: "negative timeout", mutate: func(r *UpdateOtelSettingsRequest) { r.StartupConnectivityTimeoutMs = ptr(-1) }, want: "at least 0"},
		{name: "missing node spans", mutate: func(r *UpdateOtelSettingsRequest) { r.IncludeNodeSpans = nil }, want: "includeNodeSpans"},
		{name: "missing outbound", mutate: func(r *UpdateOtelSettingsRequest) { r.InjectOutbound = nil }, want: "injectOutbound"},
		{name: "missing production only", mutate: func(r *UpdateOtelSettingsRequest) { r.ProductionExecutionsOnly = nil }, want: "productionExecutionsOnly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, otelSettingsResponse))
			request := validOtelUpdateRequest()
			tt.mutate(&request)
			_, err := server.client(t).UpdateOtelSettings(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestOtelTraceValidationBeforeTransport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*OtelTestTraceRequest)
		want   string
	}{
		{name: "invalid protocol", mutate: func(r *OtelTestTraceRequest) { r.ExporterProtocol = "http/json" }, want: "exporterProtocol"},
		{name: "missing endpoint", mutate: func(r *OtelTestTraceRequest) { r.ExporterEndpoint = nil }, want: "exporterEndpoint"},
		{name: "missing path", mutate: func(r *OtelTestTraceRequest) { r.ExporterTracingPath = nil }, want: "exporterTracingPath"},
		{name: "missing service", mutate: func(r *OtelTestTraceRequest) { r.ExporterServiceName = nil }, want: "exporterServiceName"},
		{name: "missing headers", mutate: func(r *OtelTestTraceRequest) { r.ExporterHeaders = nil }, want: "exporterHeaders"},
		{name: "missing timeout", mutate: func(r *OtelTestTraceRequest) { r.StartupConnectivityTimeoutMs = nil }, want: "startupConnectivityTimeoutMs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"success":true}`))
			request := validOtelTestRequest()
			tt.mutate(&request)
			_, err := server.client(t).TestOtelTrace(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(server.requests) != 0 {
				t.Errorf("%d invalid requests reached instance", len(server.requests))
			}
		})
	}
}

func TestOtelMutationErrorsPreserveStatusAndRedactResponse(t *testing.T) {
	const secret = "collector-super-secret"
	for _, tt := range []struct {
		name string
		call func(*Client) error
	}{
		{name: "update", call: func(client *Client) error {
			_, err := client.UpdateOtelSettings(context.Background(), validOtelUpdateRequest())
			return err
		}},
		{name: "test", call: func(client *Client) error {
			_, err := client.TestOtelTrace(context.Background(), validOtelTestRequest())
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(http.StatusBadRequest, `{"message":"bad header `+secret+`"}`))
			err := tt.call(server.client(t))
			if err == nil || StatusCodeOf(err) != http.StatusBadRequest {
				t.Fatalf("error = %v, want API status 400", err)
			}
			if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "response details redacted") {
				t.Errorf("error was not safely redacted: %v", err)
			}
		})
	}
}

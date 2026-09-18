package n8n

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

const logStreamWebhook = `{"id":"server-id","type":"webhook","label":"Audit","enabled":false,"subscribedEvents":["n8n.audit"],"anonymizeAuditMessages":false,"url":"https://user:destination-secret@example.com/path?token=destination-secret","sendHeaders":true,"headerParameters":{"parameters":[{"name":"Authorization","value":"destination-secret"},{"name":"null","value":null},{"name":"zero","value":0},{"name":"false","value":false}]},"jsonHeaders":"{\"Authorization\":\"destination-secret\"}","queryParameters":{"parameters":[{"name":"token","value":"destination-secret"}]},"jsonQuery":"destination-secret","options":{"timeout":0,"proxy":{"proxy":{"protocol":"https","host":"example.com","port":443,"password":"destination-secret"}},"future":"destination-secret"},"futureCredential":"destination-secret","circuitBreaker":{"maxFailures":0,"future":true}}`

func logStreamRequest(t *testing.T, raw string) LogStreamDestinationRequest {
	t.Helper()
	var request LogStreamDestinationRequest
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	return request
}

type logStreamCall struct {
	name, method, path, response string
	call                         func(context.Context, *Client) (any, error)
}

func logStreamCalls(t *testing.T) []logStreamCall {
	t.Helper()
	request := logStreamRequest(t, logStreamWebhook)
	const id = "id /?#"
	path := LogStreamDestinationsPath + "/id%20%2F%3F%23"
	return []logStreamCall{
		{"events", "GET", LogStreamEventTypesPath, `{"data":["n8n.workflow.started"]}`, func(ctx context.Context, c *Client) (any, error) { return c.GetLogStreamEventTypes(ctx) }},
		{"list", "GET", LogStreamDestinationsPath, `{"data":[` + logStreamWebhook + `]}`, func(ctx context.Context, c *Client) (any, error) { return c.ListLogStreamDestinations(ctx) }},
		{"create", "POST", LogStreamDestinationsPath, logStreamWebhook, func(ctx context.Context, c *Client) (any, error) { return c.CreateLogStreamDestination(ctx, request) }},
		{"get", "GET", path, logStreamWebhook, func(ctx context.Context, c *Client) (any, error) { return c.GetLogStreamDestination(ctx, id) }},
		{"update", "PUT", path, logStreamWebhook, func(ctx context.Context, c *Client) (any, error) {
			return c.UpdateLogStreamDestination(ctx, id, request)
		}},
		{"test", "POST", path + "/test", `{"success":false,"error":"destination-secret"}`, func(ctx context.Context, c *Client) (any, error) { return c.TestLogStreamDestination(ctx, id) }},
		{"delete", "DELETE", path, logStreamWebhook, func(ctx context.Context, c *Client) (any, error) { return c.DeleteLogStreamDestination(ctx, id) }},
	}
}

func TestLogStreamOperations(t *testing.T) {
	for _, tt := range logStreamCalls(t) {
		t.Run(tt.name, func(t *testing.T) {
			server := newCommunityPackageServer(t, packageJSONHandler(200, tt.response))
			result, err := tt.call(context.Background(), server.client(t))
			if err != nil {
				t.Fatal(err)
			}
			req, body := server.last(t)
			if req.Method != tt.method || req.URL.EscapedPath() != BasePath+tt.path || req.URL.RawQuery != "" {
				t.Fatalf("wrong request: %s %s", req.Method, req.URL)
			}
			if req.Header.Get(HeaderAPIKey) != "community-secret" {
				t.Error("missing API key")
			}
			if tt.name == "create" || tt.name == "update" {
				var sent map[string]any
				if err := json.Unmarshal([]byte(body), &sent); err != nil {
					t.Fatal(err)
				}
				if _, exists := sent["id"]; exists {
					t.Error("read-only ID sent")
				}
				if sent["enabled"] != false || sent["anonymizeAuditMessages"] != false || sent["futureCredential"] != "destination-secret" {
					t.Error("false or additional fields lost")
				}
				if !strings.Contains(body, "destination-secret") {
					t.Error("wire secrets were redacted")
				}
				if req.Header.Get("Content-Type") != "application/json" {
					t.Error("wrong content type")
				}
			} else if body != "" {
				t.Errorf("unexpected body: %s", body)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			for _, output := range []string{string(encoded), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", result)} {
				if strings.Contains(output, "destination-secret") {
					t.Error("response leaked secrets")
				}
			}
			switch v := result.(type) {
			case *LogStreamEventTypes:
				if len(v.Data) != 1 || v.Data[0] != "n8n.workflow.started" {
					t.Error("events not decoded")
				}
			case *LogStreamDestinations:
				if len(v.Data) != 1 || v.Data[0].Type != "webhook" {
					t.Error("list not decoded")
				}
			case *LogStreamDestination:
				if v.ID != "server-id" || v.Enabled == nil || *v.Enabled {
					t.Error("destination not decoded")
				}
			case *LogStreamTestResult:
				if v.Success {
					t.Error("false delivery not preserved")
				}
			}
		})
	}
}

func TestLogStreamVariantsAndRequestSecretSafety(t *testing.T) {
	for _, raw := range []string{
		logStreamWebhook,
		`{"type":"syslog","host":"syslog.example.com","port":514,"protocol":"tls","facility":0,"app_name":"n8n","tlsCa":"destination-secret"}`,
		`{"type":"sentry","dsn":"https://destination-secret@example.com/0"}`,
	} {
		t.Run(raw[9:15], func(t *testing.T) {
			request := logStreamRequest(t, raw)
			encoded, _ := json.Marshal(request)
			var logs strings.Builder
			slog.New(slog.NewJSONHandler(&logs, nil)).Info("request", "destination", request)
			for _, output := range []string{string(encoded), fmt.Sprintf("%+v %#v", request, request), logs.String()} {
				if strings.Contains(output, "destination-secret") {
					t.Error("request leaked secret")
				}
			}
			server := newCommunityPackageServer(t, packageJSONHandler(200, raw))
			destination, err := server.client(t).CreateLogStreamDestination(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ = json.Marshal(destination)
			if strings.Contains(string(encoded), "destination-secret") {
				t.Error("variant leaked secret")
			}
			_, body := server.last(t)
			var expected, sent map[string]any
			_ = json.Unmarshal([]byte(raw), &expected)
			delete(expected, "id")
			_ = json.Unmarshal([]byte(body), &sent)
			want, _ := json.Marshal(expected)
			got, _ := json.Marshal(sent)
			if string(got) != string(want) {
				t.Error("variant wire body differs from input")
			}
		})
	}
}

func TestLogStreamValidation(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"type":"other"}`, `{"type":"webhook"}`, `{"type":"webhook","url":"relative"}`,
		`{"type":"sentry"}`, `{"type":"sentry","dsn":"%bad"}`, `{"type":"syslog"}`, `{"type":"syslog","host":" "}`,
		`{"type":"syslog","host":"host","protocol":"http"}`, `{"type":"syslog","host":"host","facility":24}`, `{"type":"syslog","host":"host","facility":-1}`,
		`{"type":"syslog","host":"host","port":"destination-secret"}`, `{"type":"syslog","host":"host","enabled":"destination-secret"}`,
		`{"type":"webhook","url":"https://example.com","options":{"timeout":"destination-secret"}}`,
		`{"type":"webhook","url":"https://example.com","options":{"queryParameterArrays":"invalid"}}`,
		`{"type":"webhook","url":"https://example.com","options":{"proxy":{"proxy":{"protocol":"ftp"}}}}`,
		`{"type":"webhook","url":"https://example.com","headerParameters":{"parameters":[{"name":"x"}]}}`,
		`{"type":"webhook","url":"https://example.com","queryParameters":{"parameters":[{"value":null}]}}`,
		`{"type":"webhook","url":"https://example.com","headerParameters":[]}`,
		`{"type":"webhook","url":"[REDACTED]"}`, `{"type":"syslog","host":"host","future":{"key":"[REDACTED]"}}`,
		`{"type":"syslog","host":"host","future":["[REDACTED]"]}`,
		`{"type":"syslog","host":"host","protocol":""}`, `{"type":"syslog","host":"host","enabled":null}`,
		`{"type":"webhook","url":"https://example.com","options":{"queryParameterArrays":""}}`,
		`{"type":"webhook","url":"https://example.com","options":{"proxy":{"proxy":{"protocol":""}}}}`,
		`{"type":"webhook","url":"https://example.com","headerParameters":{"parameters":"destination-secret"}}`,
	} {
		var request LogStreamDestinationRequest
		err := json.Unmarshal([]byte(raw), &request)
		if err == nil {
			t.Errorf("accepted invalid input %s", raw)
		} else if strings.Contains(err.Error(), "destination-secret") {
			t.Error("validation leaked secret")
		}
	}
	server := newCommunityPackageServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached transport")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	client := server.client(t)
	for _, call := range []func() error{
		func() error {
			_, e := client.CreateLogStreamDestination(context.Background(), LogStreamDestinationRequest{})
			return e
		},
		func() error {
			_, e := client.UpdateLogStreamDestination(context.Background(), "id", LogStreamDestinationRequest{})
			return e
		},
		func() error { _, e := client.GetLogStreamDestination(context.Background(), " "); return e },
		func() error { _, e := client.DeleteLogStreamDestination(context.Background(), ""); return e },
		func() error { _, e := client.TestLogStreamDestination(context.Background(), ""); return e },
		func() error {
			_, e := client.UpdateLogStreamDestination(context.Background(), "", logStreamRequest(t, logStreamWebhook))
			return e
		},
	} {
		if call() == nil {
			t.Error("expected validation error")
		}
	}
}

func TestLogStreamErrorsEmptyAndCancellation(t *testing.T) {
	for _, tt := range logStreamCalls(t) {
		t.Run(tt.name, func(t *testing.T) {
			for _, status := range []int{400, 401, 403, 404, 409, 500, 503} {
				server := newCommunityPackageServer(t, packageJSONHandler(status, `{"message":"destination-secret","code":"destination-secret","hint":"destination-secret"}`))
				_, err := tt.call(context.Background(), server.client(t))
				if !IsStatus(err, status) {
					t.Fatalf("status lost: %v", err)
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Body != "" || strings.Contains(fmt.Sprintf("%+v", err), "destination-secret") {
					t.Error("unsafe API error")
				}
			}
			server := newCommunityPackageServer(t, packageJSONHandler(204, ""))
			if _, err := tt.call(context.Background(), server.client(t)); err != nil {
				t.Errorf("empty response: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := tt.call(ctx, server.client(t)); !errors.Is(err, context.Canceled) {
				t.Errorf("cancellation lost: %v", err)
			}
		})
	}
}

func TestLogStreamResponseSafetyAndEmptyLists(t *testing.T) {
	for _, body := range []string{
		`{"type":"webhook","destination-secret":`,
		`{"type":"webhook","options":{"destination-secret":true},"enabled":"destination-secret"}`,
	} {
		server := newCommunityPackageServer(t, packageJSONHandler(200, body))
		_, err := server.client(t).GetLogStreamDestination(context.Background(), "id")
		if err == nil || strings.Contains(err.Error(), "destination-secret") {
			t.Fatalf("expected safe decode error: %v", err)
		}
	}
	server := newCommunityPackageServer(t, packageJSONHandler(200, `{"data":null}`))
	events, err := server.client(t).GetLogStreamEventTypes(context.Background())
	if err != nil || events.Data == nil {
		t.Fatalf("events=%v err=%v", events, err)
	}
	destinations, err := server.client(t).ListLogStreamDestinations(context.Background())
	if err != nil || destinations.Data == nil {
		t.Fatalf("destinations=%v err=%v", destinations, err)
	}
}

func TestLogStreamDeliverySuccess(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(200, `{"success":true}`))
	result, err := server.client(t).TestLogStreamDestination(context.Background(), "id")
	if err != nil || !result.Success {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

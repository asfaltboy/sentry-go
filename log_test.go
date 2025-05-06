package sentry

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const (
	LogTraceID = "d49d9bf66f13450b81f65bc51cf49c03"
	LogSpanID  = "a9f442f9330b4e09"
)

var attrs = map[string]any{
	"sentry.release":              "v1.2.3",
	"sentry.environment":          "testing",
	"sentry.server.address":       "test-server",
	"sentry.trace.parent_span_id": LogSpanID,
	"sentry.sdk.name":             "sentry.go",
	"sentry.sdk.version":          "0.10.0",
}

func setupMockEvent() *MockTransport {
	mockTransport := &MockTransport{}
	mockClient, _ := NewClient(ClientOptions{
		Dsn:         testDsn,
		Transport:   mockTransport,
		Release:     "v1.2.3",
		Environment: "testing",
		ServerName:  "test-server",
	})
	mockClient.sdkIdentifier = "sentry.go"
	mockClient.sdkVersion = "0.10.0"
	hub := CurrentHub()
	hub.BindClient(mockClient)

	hub.Scope().propagationContext.TraceID = TraceIDFromHex(LogTraceID)
	hub.Scope().propagationContext.SpanID = SpanIDFromHex(LogSpanID)

	return mockTransport
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name   string
		config ClientOptions
		want   Logger
	}{
		{
			"disabled logs should return nooplogger",
			ClientOptions{},
			&noopLogger{},
		},
		{
			"enabled logs should return a new logger instance with current hub",
			ClientOptions{EnableLogs: true},
			&sentryLogger{CurrentHub()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Init(tt.config)
			if err != nil {
				t.Fatalf("cannot initialize sentry client: %e", err)
			}
			got := NewLogger(context.Background())
			assertEqual(t, got, tt.want)
		})
	}
}

func Test_sentryLogger_log(t *testing.T) {
	mockTransport := setupMockEvent()

	tests := []struct {
		name       string
		logFunc    func(l *sentryLogger, msg any)
		args       any
		wantEvents []Event
	}{
		{
			name: "Trace level",
			logFunc: func(l *sentryLogger, msg any) {
				l.Trace(msg)
			},
			args: "trace",
			wantEvents: []Event{
				{
					Logs: []Log{
						{
							TraceID:    TraceIDFromHex(LogTraceID),
							Level:      LevelTrace,
							Body:       "trace",
							Attributes: attrs,
						},
					},
				},
			},
		},
		{
			name: "Debug level",
			logFunc: func(l *sentryLogger, msg any) {
				l.Debug(msg)
			},
			args: "debug",
			wantEvents: []Event{
				{
					Logs: []Log{
						{
							TraceID:    TraceIDFromHex(LogTraceID),
							Level:      LevelDebug,
							Body:       "debug",
							Attributes: attrs,
						},
					},
				},
			},
		},
		{
			name: "Info level",
			logFunc: func(l *sentryLogger, msg any) {
				l.Info(msg)
			},
			args: "info",
			wantEvents: []Event{
				{
					Logs: []Log{
						{
							TraceID:    TraceIDFromHex(LogTraceID),
							Level:      LevelInfo,
							Body:       "info",
							Attributes: attrs,
						},
					},
				},
			},
		},
		{
			name: "Warn level",
			logFunc: func(l *sentryLogger, msg any) {
				l.Warn(msg)
			},
			args: "warn",
			wantEvents: []Event{
				{
					Logs: []Log{
						{
							TraceID:    TraceIDFromHex(LogTraceID),
							Level:      LevelWarning,
							Body:       "warn",
							Attributes: attrs,
						},
					},
				},
			},
		},
		{
			name: "Error level",
			logFunc: func(l *sentryLogger, msg any) {
				l.Error(msg)
			},
			args: "error",
			wantEvents: []Event{
				{
					Logs: []Log{
						{
							TraceID:    TraceIDFromHex(LogTraceID),
							Level:      LevelError,
							Body:       "error",
							Attributes: attrs,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &sentryLogger{
				CurrentHub(),
			}
			tt.logFunc(l, tt.args)

			opts := cmp.Options{
				cmpopts.IgnoreFields(Log{}, "Timestamp"),
			}
			gotEvents := mockTransport.Events()
			if len(gotEvents) != len(tt.wantEvents) {
				t.Fatalf("expected %d events, got %d", len(tt.wantEvents), len(gotEvents))
			}
			for i, event := range gotEvents {
				assertEqual(t, event.Type, logType)
				if diff := cmp.Diff(tt.wantEvents[i].Logs, event.Logs, opts); diff != "" {
					t.Errorf("Log mismatch (-want +got):\n%s", diff)
				}
				// pop used mock event
				mockTransport.events = mockTransport.events[1:]
			}
		})
	}
}

func Test_sentryLogger_Panic(t *testing.T) {
	t.Run("Panic level", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic, but code did not panic")
			} else {
				t.Logf("recovered panic: %v", r)
			}
		}()

		l := &sentryLogger{
			CurrentHub(),
		}
		l.Panic("panic message") // This should panic
	})
}

func Test_sentryLogger_log_Format(t *testing.T) {
	mockTransport := setupMockEvent()

	l := &sentryLogger{
		CurrentHub(),
	}
	l.Info("param matching: %v and %v", "param1", "param2")

	wantLogs := []Log{
		{
			TraceID:    TraceIDFromHex(LogTraceID),
			Level:      LevelInfo,
			Body:       "param matching: param1 and param2",
			Attributes: attrs,
		},
	}

	gotEvents := mockTransport.Events()
	if len(gotEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(gotEvents))
	}

	event := gotEvents[0]
	assertEqual(t, event.Type, logType)

	opts := cmp.Options{
		cmpopts.IgnoreFields(Log{}, "Timestamp"),
	}
	if diff := cmp.Diff(wantLogs, event.Logs, opts); diff != "" {
		t.Errorf("Log mismatch (-want +got):\n%s", diff)
	}
}

func Test_sentryLogger_Write(t *testing.T) {
	mockTransport := setupMockEvent()

	input := []byte("message from writer\n")
	wantLogs := []Log{
		{
			TraceID:    TraceIDFromHex(LogTraceID),
			Level:      LevelInfo,
			Body:       "message from writer",
			Attributes: attrs,
		},
	}

	l := &sentryLogger{
		CurrentHub(),
	}
	n, err := l.Write(input)

	if err != nil {
		t.Errorf("Write returned unexpected error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write returned wrong byte count: got %d, want %d", n, len(input))
	}

	gotEvents := mockTransport.Events()
	if len(gotEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(gotEvents))
	}
	event := gotEvents[0]
	assertEqual(t, event.Type, logType)

	opts := cmp.Options{
		cmpopts.IgnoreFields(Log{}, "Timestamp"),
	}
	if diff := cmp.Diff(wantLogs, event.Logs, opts); diff != "" {
		t.Errorf("Logs mismatch (-want +got):\n%s", diff)
	}
}

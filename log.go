package sentry

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// sentryLogger implements a custom logger that writes to Sentry
type sentryLogger struct {
	hub *Hub
}

// NewLogger returns a SentryLogger that writes to Sentry if enabled, or discards otherwise
func NewLogger() SentryLogger {
	hub := CurrentHub()
	client := hub.Client()
	if client != nil && client.options.EnableLogs {
		return &sentryLogger{hub: hub}
	}
	return &noopLogger{} // fallback: does nothing
}

func (l *sentryLogger) Write(p []byte) (n int, err error) {
	// Avoid sending double newlines to Sentry
	msg := strings.TrimRight(string(p), "\n")
	err = l.log(LevelInfo, msg)
	return len(p), err
}

func (l *sentryLogger) log(level Level, args ...interface{}) error {
	if len(args) == 0 {
		return nil
	}

	event := NewEvent()
	event.Timestamp = time.Now()
	event.Type = logType
	traceParent := l.hub.GetTraceparent()
	traceID := traceParent[:32]
	if traceParent != "" {
		if event.Contexts == nil {
			event.Contexts = make(map[string]Context)
		}
		event.Contexts["trace"] = map[string]interface{}{
			"trace_id": traceID,
		}
	}

	var template string
	var message string
	var parameters []interface{}
	attrs := map[string]any{}

	for _, arg := range args {
		switch a := arg.(type) {
		case string:
			if template == "" {
				template = a
			} else {
				parameters = append(parameters, a)
			}
		//case attribute.Builder:
		//	for k, v := range a {
		//		attrs[k] = v
		//	}
		default:
			parameters = append(parameters, a)
		}
	}

	if template != "" && len(parameters) > 0 {
		message = fmt.Sprintf(template, parameters...)
	} else {
		message = template
	}
	if template != "" && len(parameters) > 0 {
		attrs["sentry.message.template"] = Attribute{
			Value: template, Type: "string",
		}
		for i, p := range parameters {
			attrs[fmt.Sprintf("sentry.message.parameters.%d", i)] = Attribute{
				Value: fmt.Sprint(p), Type: "string",
			}
		}
	}

	// handle metadata
	client := l.hub.Client()
	if release := client.options.Release; release != "" {
		attrs["sentry.release"] = release
	}
	if environment := client.options.Environment; environment != "" {
		attrs["sentry.environment"] = environment
	}
	if serverAddr := client.options.ServerName; serverAddr != "" {
		attrs["sentry.server.address"] = serverAddr
	}
	if traceParent != "" {
		attrs["sentry.trace.parent_span_id"] = traceParent[34:]
	}
	if sdkIdentifier := client.sdkIdentifier; sdkIdentifier != "" {
		attrs["sentry.sdk.name"] = sdkIdentifier
	}
	if sdkVersion := client.sdkVersion; sdkVersion != "" {
		attrs["sentry.sdk.version"] = sdkVersion

	}

	event.Logs = []Log{
		{
			Timestamp:  time.Now(),
			TraceID:    TraceID([]byte(traceID)),
			Level:      level,
			Body:       message,
			Attributes: attrs,
		},
	}

	l.hub.CaptureEvent(event)
	return nil
}

func (l *sentryLogger) Trace(v ...interface{}) { _ = l.log(LevelTrace, v...) }
func (l *sentryLogger) Debug(v ...interface{}) { _ = l.log(LevelDebug, v...) }
func (l *sentryLogger) Info(v ...interface{})  { _ = l.log(LevelInfo, v...) }
func (l *sentryLogger) Warn(v ...interface{})  { _ = l.log(LevelWarning, v...) }
func (l *sentryLogger) Error(v ...interface{}) { _ = l.log(LevelError, v...) }
func (l *sentryLogger) Fatal(v ...interface{}) { _ = l.log(LevelFatal, v...); os.Exit(1) }
func (l *sentryLogger) Panic(v ...interface{}) { _ = l.log(LevelFatal, v...); panic(fmt.Sprint(v...)) }

// fallback no-op logger if Sentry is not enabled
type noopLogger struct{}

func (*noopLogger) Trace(v ...interface{}) {}
func (*noopLogger) Debug(v ...interface{}) {}
func (*noopLogger) Info(v ...interface{})  {}
func (*noopLogger) Warn(v ...interface{})  {}
func (*noopLogger) Error(v ...interface{}) {}
func (*noopLogger) Fatal(v ...interface{}) { os.Exit(1) }
func (*noopLogger) Panic(v ...interface{}) { panic("invalid setup: EnableLogs disabled") }
func (*noopLogger) Write(p []byte) (n int, err error) {
	return 0, errors.New("invalid setup: EnableLogs disabled")
}

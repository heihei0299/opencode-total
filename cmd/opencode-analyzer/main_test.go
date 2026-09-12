package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

type mainRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn mainRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func mainRPCResponse(value any) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(opencode.EncodePayload([]any{value}))),
		Header:     make(http.Header),
	}
}

func captureMainOutput(run func() error) (string, error) {
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = previous
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		return string(output), readErr
	}
	return string(output), runErr
}

func TestRunSyncReportsFailedStatus(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = mainRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("failed")), Header: make(http.Header)}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	output, err := captureMainOutput(func() error {
		return runSync([]string{"--data-dir", t.TempDir()})
	})
	if err == nil {
		t.Fatalf("failed sync must return an error; output=%s", output)
	}
	if !strings.Contains(output, "failed") {
		t.Fatalf("failed sync output = %s", output)
	}
}

func TestRunSyncReportsPartialAndReturnsError(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = mainRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Header.Get("X-Server-Id") {
		case opencode.FnUsageHistory:
			return mainRPCResponse([]opencode.UsageRecord{}), nil
		case opencode.FnMonthlyCosts:
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("temporary failure")), Header: make(http.Header)}, nil
		default:
			return nil, errors.New("unexpected RPC")
		}
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	output, err := captureMainOutput(func() error {
		return runSync([]string{"--data-dir", t.TempDir()})
	})
	if err == nil {
		t.Fatalf("partial sync must return a non-zero error; output=%s", output)
	}
	if !strings.Contains(output, "partial") || !strings.Contains(output, "月度成本未更新") {
		t.Fatalf("partial sync output = %s", output)
	}
}

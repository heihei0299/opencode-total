package opencode

import (
	"testing"
)

func TestSerovalEncodingAndDecoding(t *testing.T) {
	// 1. EncodePayload
	payload := EncodePayload([]any{"wrk_123", 1})
	if payload == "" {
		t.Fatalf("expected non-empty payload")
	}

	// 2. Decode Seroval Envelope
	envelope := `{"t":{"t":9,"i":0,"l":1,"a":[{"id":"test_1","model":"gpt-4"}]},"f":31,"m":[]}`
	decoded, err := DecodeStreamChunk(envelope)
	if err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	m, ok := decoded.(map[string]any)
	if !ok || m["id"] != "test_1" {
		t.Fatalf("expected unwrapped data with id=test_1, got %+v", decoded)
	}

	// 3. Decode SolidStart JS chunk with $R assignments
	jsChunk := `self.$R[0]=[$R[1]={"id":"usg_1","inputTokens":100},$R[2]={"id":"usg_2","inputTokens":200}])($R)`
	res, err := DecodeResponseText(jsChunk)
	if err != nil {
		t.Fatalf("failed to decode SolidStart JS chunk: %v", err)
	}
	arr, ok := res.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected array of 2 elements from JS chunk, got %+v", res)
	}
	first := arr[0].(map[string]any)
	if first["id"] != "usg_1" {
		t.Errorf("expected usg_1, got %v", first["id"])
	}
}

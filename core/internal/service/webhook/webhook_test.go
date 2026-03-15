package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComputeHMAC(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		secret  string
	}{
		{
			name:    "basic payload",
			payload: []byte(`{"event":"open","timestamp":1234567890}`),
			secret:  "test-secret-key",
		},
		{
			name:    "empty secret",
			payload: []byte(`{"event":"click"}`),
			secret:  "",
		},
		{
			name:    "complex payload",
			payload: []byte(`{"event":"bounce","data":{"recipient":"user@example.com","dsn":"5.1.1"}}`),
			secret:  "my-super-secret-key-123!@#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := computeHMAC(tt.payload, tt.secret)

			// Verify format
			if len(sig) < 7 || sig[:7] != "sha256=" {
				t.Errorf("computeHMAC() signature should start with 'sha256=', got %s", sig)
			}

			// Verify the HMAC value manually
			mac := hmac.New(sha256.New, []byte(tt.secret))
			mac.Write(tt.payload)
			expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

			if sig != expected {
				t.Errorf("computeHMAC() = %s, want %s", sig, expected)
			}
		})
	}
}

func TestContainsEvent(t *testing.T) {
	tests := []struct {
		name   string
		events []string
		event  string
		want   bool
	}{
		{
			name:   "event present",
			events: []string{"open", "click", "bounce"},
			event:  "click",
			want:   true,
		},
		{
			name:   "event absent",
			events: []string{"open", "click"},
			event:  "bounce",
			want:   false,
		},
		{
			name:   "empty events",
			events: []string{},
			event:  "open",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsEvent(tt.events, tt.event); got != tt.want {
				t.Errorf("containsEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			s:      "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			s:      "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string",
			s:      "hello world",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "empty string",
			s:      "",
			maxLen: 5,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.s, tt.maxLen); got != tt.want {
				t.Errorf("truncate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllEventTypes(t *testing.T) {
	types := AllEventTypes()
	if len(types) == 0 {
		t.Error("AllEventTypes() returned empty slice")
	}

	expectedTypes := map[string]bool{
		EventDelivery:    false,
		EventBounce:      false,
		EventOpen:        false,
		EventClick:       false,
		EventUnsubscribe: false,
		EventComplaint:   false,
		EventDeferral:    false,
	}

	for _, typ := range types {
		if _, exists := expectedTypes[typ]; !exists {
			t.Errorf("Unexpected event type: %s", typ)
		}
		expectedTypes[typ] = true
	}

	for typ, found := range expectedTypes {
		if !found {
			t.Errorf("Missing expected event type: %s", typ)
		}
	}
}

func TestSendWebhook(t *testing.T) {
	// Create test server
	var receivedBody string
	var receivedSig string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		receivedSig = r.Header.Get("X-BillionMail-Signature")
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	payload := WebhookPayload{
		Event:     EventOpen,
		Timestamp: 1234567890,
		Data: map[string]interface{}{
			"recipient":  "test@example.com",
			"message_id": "abc-123",
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	secret := "test-secret"

	statusCode, body, err := sendWebhook(server.URL, secret, payloadBytes)
	if err != nil {
		t.Fatalf("sendWebhook() error = %v", err)
	}

	if statusCode != 200 {
		t.Errorf("sendWebhook() statusCode = %d, want 200", statusCode)
	}

	if body != `{"status":"ok"}` {
		t.Errorf("sendWebhook() body = %s, want {\"status\":\"ok\"}", body)
	}

	// Verify content type
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", receivedContentType)
	}

	// Verify the payload was received correctly
	if receivedBody != string(payloadBytes) {
		t.Errorf("Received body = %s, want %s", receivedBody, string(payloadBytes))
	}

	// Verify HMAC signature
	expectedSig := computeHMAC(payloadBytes, secret)
	if receivedSig != expectedSig {
		t.Errorf("Received signature = %s, want %s", receivedSig, expectedSig)
	}
}

func TestSendWebhookWithoutSecret(t *testing.T) {
	var receivedSig string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-BillionMail-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := []byte(`{"event":"test"}`)
	statusCode, _, err := sendWebhook(server.URL, "", payload)
	if err != nil {
		t.Fatalf("sendWebhook() error = %v", err)
	}

	if statusCode != 200 {
		t.Errorf("sendWebhook() statusCode = %d, want 200", statusCode)
	}

	// When no secret, X-BillionMail-Signature header should be empty
	if receivedSig != "" {
		t.Errorf("X-BillionMail-Signature should be empty when no secret, got %s", receivedSig)
	}
}

func TestSendWebhookServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	payload := []byte(`{"event":"test"}`)
	statusCode, body, err := sendWebhook(server.URL, "secret", payload)
	if err != nil {
		t.Fatalf("sendWebhook() should not return error for HTTP errors, got %v", err)
	}

	if statusCode != 500 {
		t.Errorf("sendWebhook() statusCode = %d, want 500", statusCode)
	}

	if body != "server error" {
		t.Errorf("sendWebhook() body = %s, want 'server error'", body)
	}
}

func TestWebhookPayloadSerialization(t *testing.T) {
	payload := WebhookPayload{
		Event:     EventBounce,
		Timestamp: 1234567890,
		Data: map[string]interface{}{
			"recipient":   "user@example.com",
			"description": "mailbox full",
		},
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded WebhookPayload
	err = json.Unmarshal(bytes, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.Event != EventBounce {
		t.Errorf("Event = %s, want %s", decoded.Event, EventBounce)
	}

	if decoded.Timestamp != 1234567890 {
		t.Errorf("Timestamp = %d, want 1234567890", decoded.Timestamp)
	}
}

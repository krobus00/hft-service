package infrastructure

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPServerAddsCORSHeaders(t *testing.T) {
	server := NewHTTPServerWithConfig(HTTPServerConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin '*', got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type, X-API-Key, X-Request-Id" {
		t.Fatalf("unexpected Access-Control-Allow-Headers: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "X-Request-Id" {
		t.Fatalf("unexpected Access-Control-Expose-Headers: %q", got)
	}
}

func TestHTTPServerHandlesCORSPreflight(t *testing.T) {
	called := false
	server := NewHTTPServerWithConfig(HTTPServerConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if called {
		t.Fatal("expected preflight request to stop before the wrapped handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatalf("unexpected Access-Control-Allow-Methods: %q", got)
	}
}

func TestHTTPServerPassesThroughNonCORSOptions(t *testing.T) {
	called := false
	server := NewHTTPServerWithConfig(HTTPServerConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if !called {
		t.Fatal("expected non-CORS OPTIONS request to reach the wrapped handler")
	}
}

func TestHTTPResponseRecorderSupportsHijack(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	writer := &hijackResponseWriter{conn: serverConn}
	recorder := &httpResponseRecorder{ResponseWriter: writer, statusCode: http.StatusOK}

	conn, _, err := recorder.Hijack()
	if err != nil {
		t.Fatalf("expected hijack to succeed: %v", err)
	}
	defer conn.Close()
	if recorder.statusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected status %d, got %d", http.StatusSwitchingProtocols, recorder.statusCode)
	}
}

type hijackResponseWriter struct {
	conn net.Conn
}

func (w *hijackResponseWriter) Header() http.Header {
	return http.Header{}
}

func (w *hijackResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *hijackResponseWriter) WriteHeader(int) {}

func (w *hijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

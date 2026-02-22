package infrastructure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/sirupsen/logrus"
)

const (
	defaultHTTPAddr         = ":8080"
	defaultReadTimeout      = 5 * time.Second
	defaultReadHeaderTimeout = 2 * time.Second
	defaultWriteTimeout     = 15 * time.Second
	defaultIdleTimeout      = 60 * time.Second
	defaultShutdownTimeout  = 10 * time.Second
	defaultMaxHeaderBytes   = 1 << 20
)

type HTTPServer struct {
	server          *http.Server
	shutdownTimeout time.Duration
}

type HTTPServerConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

func DefaultHTTPServerConfig() HTTPServerConfig {
	return HTTPServerConfig{
		Addr:              resolveHTTPAddr(),
		ReadTimeout:       defaultReadTimeout,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		ShutdownTimeout:   defaultShutdownTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
	}
}

func NewHTTPServer() *HTTPServer {
	return NewHTTPServerWithConfig(DefaultHTTPServerConfig(), nil)
}

func NewHTTPServerWithConfig(cfg HTTPServerConfig, handler http.Handler) *HTTPServer {
	if handler == nil {
		handler = defaultHTTPMux()
	}

	withMiddlewares := chainHTTPMiddleware(
		handler,
		httpRequestIDMiddleware,
		httpRecoveryMiddleware,
		httpSecurityHeadersMiddleware,
		httpAccessLogMiddleware,
	)

	if cfg.Addr == "" {
		cfg.Addr = defaultHTTPAddr
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}
	if cfg.ReadHeaderTimeout <= 0 {
		cfg.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultIdleTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	if cfg.MaxHeaderBytes <= 0 {
		cfg.MaxHeaderBytes = defaultMaxHeaderBytes
	}

	return &HTTPServer{
		server: &http.Server{
			Addr:              cfg.Addr,
			Handler:           withMiddlewares,
			ReadTimeout:       cfg.ReadTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			MaxHeaderBytes:    cfg.MaxHeaderBytes,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

func (h *HTTPServer) Start() error {
	logrus.WithField("addr", h.server.Addr).Info("http server starting")
	err := h.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (h *HTTPServer) Shutdown(ctx context.Context) error {
	shutdownCtx := ctx
	if shutdownCtx == nil {
		innerCtx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
		defer cancel()
		shutdownCtx = innerCtx
	}

	return h.server.Shutdown(shutdownCtx)
}

func (h *HTTPServer) Handler() http.Handler {
	return h.server.Handler
}

func defaultHTTPMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	return mux
}

type httpMiddleware func(http.Handler) http.Handler

func chainHTTPMiddleware(handler http.Handler, middlewares ...httpMiddleware) http.Handler {
	wrapped := handler
	for idx := len(middlewares) - 1; idx >= 0; idx-- {
		wrapped = middlewares[idx](wrapped)
	}

	return wrapped
}

func httpRequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = newRequestID()
		}

		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r)
	})
}

func httpSecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func httpRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logrus.WithFields(logrus.Fields{
					"method": r.Method,
					"path":   r.URL.Path,
					"panic":  recovered,
				}).Error("panic recovered in http handler")

				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal server error"))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func httpAccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &httpResponseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(writer, r)

		logrus.WithFields(logrus.Fields{
			"method":      r.Method,
			"path":        r.URL.Path,
			"remote_addr": clientIPFromRequest(r),
			"status":      writer.statusCode,
			"duration_ms": time.Since(started).Milliseconds(),
		}).Info("http request handled")
	})
}

type httpResponseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *httpResponseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	return hex.EncodeToString(b)
}

func clientIPFromRequest(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-Ip")); realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}

func resolveHTTPAddr() string {
	if addr := strings.TrimSpace(os.Getenv("HTTP_ADDR")); addr != "" {
		return addr
	}

	if port := strings.TrimSpace(os.Getenv("HTTP_PORT")); port != "" {
		if strings.HasPrefix(port, ":") {
			return port
		}

		return ":" + port
	}

	if config.Env != nil {
		if port := strings.TrimSpace(config.Env.Port["http"]); port != "" {
			if strings.HasPrefix(port, ":") {
				return port
			}

			return ":" + port
		}
	}

	return defaultHTTPAddr
}

package http

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	apiutil "github.com/krobus00/hft-service/internal/api"
	"github.com/krobus00/hft-service/internal/constant"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func (h *Handler) StrategyMetricDetailWS(w nethttp.ResponseWriter, r *nethttp.Request) {
	if !websocketAuthorized(w, r, h) {
		return
	}
	filter, ok := parseStrategyMetricFilter(w, r)
	if !ok {
		return
	}
	conn, _, ok := acceptWebSocket(w, r)
	if !ok {
		return
	}
	defer conn.Close()

	interval := websocketInterval(r.URL.Query().Get("refresh"))
	send := func() bool {
		result, err := h.dataService.GetStrategyMetricDetail(r.Context(), filter)
		if err != nil {
			return writeWebSocketJSON(conn, map[string]string{"error": err.Error()}) == nil
		}
		return writeWebSocketJSON(conn, result) == nil
	}
	if !send() {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

func websocketAuthorized(w nethttp.ResponseWriter, r *nethttp.Request, h *Handler) bool {
	raw := strings.TrimSpace(r.URL.Query().Get("token"))
	if raw == "" {
		apiutil.WriteError(w, nethttp.StatusUnauthorized, constant.UnauthorizedStatusCode, "authorization token is required")
		return false
	}
	claims, err := apiutil.VerifyToken(raw, h.authConfig.TokenSecret)
	if err != nil || claims == nil || strings.TrimSpace(claims.Subject) == "" {
		apiutil.WriteError(w, nethttp.StatusUnauthorized, constant.UnauthorizedStatusCode, "invalid authorization token")
		return false
	}
	user, err := h.authService.Me(r.Context(), claims.Subject)
	if err != nil {
		writeAuthAccessError(w, err)
		return false
	}
	claims.Roles = user.Roles
	claims.Permissions = user.Permissions
	if !hasPermission(claims, constant.PermissionStrategyConfigRead) {
		apiutil.WriteError(w, nethttp.StatusForbidden, constant.ForbiddenStatusCode, "permission denied")
		return false
	}
	return true
}

func acceptWebSocket(w nethttp.ResponseWriter, r *nethttp.Request) (net.Conn, *bufio.ReadWriter, bool) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		apiutil.WriteError(w, nethttp.StatusBadRequest, constant.BadRequestStatusCode, "websocket upgrade required")
		return nil, nil, false
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		apiutil.WriteError(w, nethttp.StatusBadRequest, constant.BadRequestStatusCode, "websocket key is required")
		return nil, nil, false
	}
	hijacker, ok := w.(nethttp.Hijacker)
	if !ok {
		apiutil.WriteError(w, nethttp.StatusInternalServerError, constant.InternalStatusCode, "websocket unsupported")
		return nil, nil, false
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, false
	}
	accept := websocketAccept(key)
	_, err = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err != nil || rw.Flush() != nil {
		_ = conn.Close()
		return nil, nil, false
	}
	return conn, rw, true
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebSocketJSON(conn net.Conn, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 127, 0, 0, 0, 0, byte(len(payload)>>24), byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

func websocketInterval(raw string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || seconds <= 0 {
		return 5 * time.Second
	}
	if seconds < 2 {
		seconds = 2
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

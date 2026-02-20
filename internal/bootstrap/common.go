package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type operation func(ctx context.Context) error

// gracefulShutdown waits for termination syscalls and doing clean up operations after received it.
func gracefulShutdown(ctx context.Context, timeout time.Duration, ops map[string]operation) <-chan struct{} {
	wait := make(chan struct{})
	go func() {
		s := make(chan os.Signal, 1)

		// add any other syscalls that you want to be notified with
		signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		<-s

		logrus.Info("shutting down")

		// set timeout for the ops to be done to prevent system hang
		timeoutFunc := time.AfterFunc(timeout, func() {
			logrus.Error(fmt.Sprintf("timeout %d ms has been elapsed, force exit", timeout.Milliseconds()))
			os.Exit(0)
		})

		defer timeoutFunc.Stop()

		var wg sync.WaitGroup

		// Do the operations asynchronously to save time
		for key, op := range ops {
			wg.Add(1)
			innerOp := op
			innerKey := key
			go func() {
				defer wg.Done()

				logrus.Info(fmt.Sprintf("cleaning up: %s", innerKey))
				if err := innerOp(ctx); err != nil {
					logrus.Error(fmt.Sprintf("%s: clean up failed: %s", innerKey, err.Error()))
					return
				}

				logrus.Info(fmt.Sprintf("%s was shutdown gracefully", innerKey))
			}()
		}

		wg.Wait()

		close(wait)
	}()

	return wait
}

func runWS(ctx context.Context, wsHost url.URL, initSub map[string]any, onMessage func(ctx context.Context, message []byte) error) (*websocket.Conn, error) {

	logrus.Infof("connecting to %s", wsHost.String())

	c, _, err := websocket.DefaultDialer.Dial(wsHost.String(), nil)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}

	// Setup pong handler
	c.SetPongHandler(func(string) error {
		logrus.Info("pong")
		return nil
	})

	if err := c.WriteJSON(initSub); err != nil {
		return nil, err
	}

	// Ping loop (optional but recommended)
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := c.WriteMessage(websocket.PingMessage, nil)
				if err != nil {
					logrus.Error(err)
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Read loop
	for {
		select {
		case <-ctx.Done():
			return c, nil
		default:
			_, message, err := c.ReadMessage()
			if err != nil {
				logrus.Error(err)
				return c, err
			}

			if onMessage != nil {
				if err := onMessage(ctx, message); err != nil {
					logrus.Error(err)
				}
			}
		}
	}
}

func parseClosedKlinePrice(message []byte) (float64, bool) {
	var payload struct {
		Event string `json:"e"`
		Kline struct {
			Close    string `json:"c"`
			IsClosed bool   `json:"x"`
		} `json:"k"`
	}

	if err := json.Unmarshal(message, &payload); err != nil {
		return 0, false
	}

	if payload.Event != "kline" || payload.Kline.Close == "" || !payload.Kline.IsClosed {
		return 0, false
	}

	price, err := strconv.ParseFloat(payload.Kline.Close, 64)
	if err != nil {
		return 0, false
	}

	return price, true
}

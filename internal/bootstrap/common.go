package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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

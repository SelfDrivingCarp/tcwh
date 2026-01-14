package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/selfdrivingcarp/cmdutils/fatal"
	"github.com/selfdrivingcarp/tcwh"
	"github.com/selfdrivingcarp/tcwh/controller"
	"github.com/selfdrivingcarp/tcwh/server"
)

func main() {
	fatal.IfF(len(os.Args) != 2, "usage: %s <config path>", os.Args[0])

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := tcwh.LoadConfig(os.Args[1])
	fatal.IfErrorF(err, "loading config: %v")
	fatal.IfErrorF(cfg.Validate(), "validating config: %v")

	logLevel := slog.LevelInfo
	if cfg.LogDebug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	logger.Info("logging", "level", logLevel)

	ctrl, err := controller.New(cfg, logger)
	fatal.IfErrorF(err, "creating controller: %v")
	defer ctrl.Close()

	handler, err := server.New(cfg, logger, ctrl)
	fatal.IfErrorF(err, "creating server: %w")

	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: handler,
	}
	go func() {
		<-ctx.Done()
		logger.Info("shutting down server")
		if err := srv.Shutdown(context.Background()); err != nil {
			logger.Error("shutting down server", "error", err.Error())
		}
	}()

	wg := sync.WaitGroup{}

	wg.Go(func() {
		logger.Info("listening", "address", cfg.Listen)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listening", "address", cfg.Listen, "error", err.Error())
		}
	})

	logger.Info("waiting for listener")
	for {
		if ctx.Err() != nil {
			break
		}
		time.Sleep(time.Millisecond * 500)
		logger.Debug("checking for web service")
		if _, err := http.Get(cfg.Twitch.WebhookCBURL); err == nil {
			break
		}
	}

	ctrl.Start(ctx)

	wg.Wait()
}

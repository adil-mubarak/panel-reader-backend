package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"panel-reader/backend/internal/app"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	storageRoot := envOr("PANEL_READER_STORAGE", "../storage")
	databasePath := envOr("PANEL_READER_DATABASE", filepath.Join(storageRoot, "panel-reader.db"))
	detectorTimeout := envDuration("PANEL_READER_AI_TIMEOUT", 30*time.Second, logger)

	application, err := app.New(app.Config{
		StorageRoot:          storageRoot,
		DatabasePath:         databasePath,
		MaxUpload:            1 << 30,
		MaxEntries:           2000,
		MaxExtracted:         4 << 30,
		MaxFile:              100 << 20,
		PanelDetectorURL:     os.Getenv("PANEL_READER_AI_URL"),
		PanelDetectorTimeout: detectorTimeout,
		PanelDetectorRoot:    os.Getenv("PANEL_READER_AI_STORAGE_ROOT"),
	}, logger)
	if err != nil {
		logger.Error("initialize application", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	server := &http.Server{
		Addr:              envOr("PANEL_READER_ADDR", ":8080"),
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Minute,
		WriteTimeout:      20 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		logger.Info("server listening", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}

func envDuration(name string, fallback time.Duration, logger *slog.Logger) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		logger.Warn("invalid duration environment variable", "name", name, "value", value)
		return fallback
	}
	return duration
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

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

	application, err := app.New(app.Config{
		StorageRoot:  storageRoot,
		DatabasePath: databasePath,
		MaxUpload:    1 << 30,
		MaxEntries:   2000,
		MaxExtracted: 4 << 30,
		MaxFile:      100 << 20,
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

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

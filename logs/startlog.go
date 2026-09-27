// Copyright (c) 2026 Михаил Попов
// ./maintenance_1C/logs/startlog.go
package logs

import (
	"io"
	"log/slog"
)

var Logger *slog.Logger

func Startlog(file io.Writer) *slog.Logger {
	handler := slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
	Logger = slog.New(handler)
	return Logger
}

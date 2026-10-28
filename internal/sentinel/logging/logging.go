// Package logging configures the structured logger used by the Sentinel daemon.
package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Format identifies the output format of the log records.
type Format string

const (
	// FormatText writes human-readable log records.
	FormatText Format = "text"

	// FormatJSON writes machine-readable log records.
	FormatJSON Format = "json"
)

// Config holds the configuration of the Sentinel logger.
type Config struct {

	// Level is the minimum severity of the emitted records.
	Level slog.Level

	// Format is the output format of the emitted records.
	Format Format
}

// parseLevelNames lists the level names accepted by the log level parser.
var parseLevelNames = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// ParseLevel converts a level name into a slog.Level. The name is matched
// case-insensitively after trimming surrounding whitespace, and must be one
// of debug, info, warn, or error.
func ParseLevel(raw string) (slog.Level, error) {
	level, ok := parseLevelNames[strings.ToLower(strings.TrimSpace(raw))]
	if !ok {
		return 0, fmt.Errorf("invalid log level %q: must be one of debug, info, warn, error", raw)
	}

	return level, nil
}

// ParseFormat converts a format name into a Format. The name is matched
// case-insensitively after trimming surrounding whitespace, and must be one
// of text or json.
func ParseFormat(raw string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(FormatText):
		return FormatText, nil
	case string(FormatJSON):
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("invalid log format %q: must be one of text or json", raw)
	}
}

// New creates a logger that writes records to the provided writer using the
// provided configuration. An unrecognized format falls back to the text
// handler.
func New(w io.Writer, cfg Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.Level}

	if cfg.Format == FormatJSON {
		return slog.New(slog.NewJSONHandler(w, opts))
	}

	return slog.New(slog.NewTextHandler(w, opts))
}

// Fingerprint returns a short, non-reversible identifier of the provided
// secret, so that log records can correlate it without revealing it.
func Fingerprint(secret string) string {
	digest := sha256.Sum256([]byte(secret))

	return hex.EncodeToString(digest[:])[:8]
}

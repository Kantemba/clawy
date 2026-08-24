package logger

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Kantemba/clawy/pkg/security"
	"github.com/rs/zerolog"
	"golang.org/x/term"
)

type LogLevel = zerolog.Level

const (
	DEBUG = zerolog.DebugLevel
	INFO  = zerolog.InfoLevel
	WARN  = zerolog.WarnLevel
	ERROR = zerolog.ErrorLevel
	FATAL = zerolog.FatalLevel

	Component = "component"
)

var (
	logLevelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
		FATAL: "FATAL",
	}

	currentLevel  = INFO
	logger        zerolog.Logger
	logFile       *os.File
	once          sync.Once
	mu            sync.RWMutex
	writers       []io.Writer
	consoleWriter zerolog.ConsoleWriter
	redactor      *security.Redactor
)

func init() {
	once.Do(func() {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

		isTTY := term.IsTerminal(int(os.Stdout.Fd()))

		consoleWriter = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05", // TODO: make it configurable???

			// Custom formatter to handle multiline strings and JSON objects
			FormatFieldValue: formatFieldValue,
			PartsOrder: []string{
				zerolog.TimestampFieldName,
				zerolog.LevelFieldName,
				Component,
				zerolog.CallerFieldName,
				zerolog.MessageFieldName,
			},
			FieldsExclude: []string{Component},
			FormatPrepare: func(fields map[string]any) error {
				if isTTY {
					fields[Component] = fmt.Sprintf("\x1b[33m%v\x1b[0m", fields[Component])
				}
				return nil
			},
			NoColor: !isTTY,
		}

		writers = append(writers, consoleWriter)

		logger = zerolog.New(io.MultiWriter(writers...)).With().Timestamp().Caller().Logger()

		// Initialize the security redactor for log output.
		redactor = security.NewRedactor()
	})
}

func formatFieldValue(i any) string {
	var s string

	switch val := i.(type) {
	case string:
		s = val
	case []byte:
		s = string(val)
	default:
		return fmt.Sprintf("%v", i)
	}

	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}

	if strings.Contains(s, "\n") {
		return fmt.Sprintf("\n%s", s)
	}

	if strings.Contains(s, " ") {
		if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
			(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
			return s
		}
		return fmt.Sprintf("%q", s)
	}

	return s
}

func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
	zerolog.SetGlobalLevel(level)
}

func SetConsoleLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	logger = logger.Level(level)
}

func DisableConsole() {
	mu.Lock()
	defer mu.Unlock()
	writers[0] = io.Discard
	logger = logger.Output(io.MultiWriter(writers...))
}

func EnableConsole() {
	mu.Lock()
	defer mu.Unlock()
	writers[0] = consoleWriter
	logger = logger.Output(io.MultiWriter(writers...))
}

func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}

// ParseLevel converts a case-insensitive level name to a LogLevel.
// Returns the level and true if valid, or (INFO, false) if unrecognized.
func ParseLevel(s string) (LogLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return DEBUG, true
	case "info":
		return INFO, true
	case "warn", "warning":
		return WARN, true
	case "error":
		return ERROR, true
	case "fatal":
		return FATAL, true
	default:
		return INFO, false
	}
}

// SetLevelFromString sets the log level from a string value.
// If the string is empty or not a recognized level name, the current level is kept.
func SetLevelFromString(s string) {
	if s == "" {
		return
	}
	if level, ok := ParseLevel(s); ok {
		SetLevel(level)
	}
}

func EnableFileLogging(filePath string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	newFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Close old file if exists
	if logFile != nil {
		logFile.Close()
	}

	logFile = newFile

	if len(writers) != 1 {
		return fmt.Errorf("failed to configure file logging: %w", err)
	}

	writers = append(writers, logFile)
	logger = logger.Output(io.MultiWriter(writers...))

	return nil
}

func DisableFileLogging() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	if len(writers) > 1 {
		writers = writers[:1]
		logger = logger.Output(io.MultiWriter(writers...))
	}
}

// AttachSink registers an additional writer (e.g. an OpenTelemetry log
// bridge) alongside the console and file outputs.
func AttachSink(w io.Writer) {
	if w == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	for _, existing := range writers {
		if existing == w {
			return
		}
	}
	writers = append(writers, w)
	logger = logger.Output(io.MultiWriter(writers...))
}

// DetachSink removes a previously attached writer.
func DetachSink(w io.Writer) {
	if w == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	filtered := make([]io.Writer, 0, len(writers))
	for _, existing := range writers {
		if existing != w {
			filtered = append(filtered, existing)
		}
	}
	if len(filtered) == len(writers) {
		return
	}
	writers = filtered
	logger = logger.Output(io.MultiWriter(writers...))
}

func ConfigureFromEnv() {
	if logFile := os.Getenv("CLAWY_LOG_FILE"); logFile != "" {
		if strings.HasPrefix(logFile, "~/") {
			if home := os.Getenv("HOME"); home != "" {
				logFile = filepath.Join(home, logFile[2:])
			}
		}

		if err := EnableFileLogging(logFile); err != nil {
			fmt.Fprintf(os.Stderr, "failed to enable file logging: %v\n", err)
		} else {
			DisableConsole()
		}
	}
}

const (
	locUnknown = "<unknown>"
)

func getPackageNameFromFile(filePath string) string {
	dir := filepath.Dir(filePath)
	importPath := filepath.ToSlash(dir)

	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return locUnknown
	}

	pkg := parts[len(parts)-1]
	if pkg == "." {
		return "<main>"
	}

	return pkg
}

func getCallerSkip() (int, string) {
	for i := 2; i < 15; i++ {
		pc, file, _, ok := runtime.Caller(i)
		if !ok {
			continue
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		// bypass common loggers
		if strings.HasSuffix(file, "/logger.go") ||
			strings.HasSuffix(file, "/logger_3rd_party.go") ||
			strings.HasSuffix(file, "/log.go") {
			continue
		}

		funcName := fn.Name()
		if strings.HasPrefix(funcName, "runtime.") {
			continue
		}

		return i - 1, getPackageNameFromFile(file)
	}

	return 3, locUnknown
}

//nolint:zerologlint
func getEvent(logger zerolog.Logger, level LogLevel) *zerolog.Event {
	switch level {
	case zerolog.DebugLevel:
		return logger.Debug()
	case zerolog.InfoLevel:
		return logger.Info()
	case zerolog.WarnLevel:
		return logger.Warn()
	case zerolog.ErrorLevel:
		return logger.Error()
	case zerolog.FatalLevel:
		return logger.Fatal()
	default:
		return logger.Info()
	}
}

// SetRedactor sets a custom security redactor for log output.
// Pass nil to disable redaction.
func SetRedactor(r *security.Redactor) {
	mu.Lock()
	defer mu.Unlock()
	redactor = r
}

// GetRedactor returns the current security redactor.
func GetRedactor() *security.Redactor {
	mu.RLock()
	defer mu.RUnlock()
	return redactor
}

func logMessage(level LogLevel, component string, message string, fields map[string]any) {
	if level < currentLevel {
		return
	}

	skip, pkg := getCallerSkip()

	event := getEvent(logger, level)

	if component == "" {
		component = pkg
	}

	event.Str(Component, component)

	// Apply security redaction to string fields.
	if redactor != nil {
		fields = redactor.RedactMap(fields)
	}

	appendFields(event, fields)

	event.CallerSkipFrame(skip).Msg(message)
}

func appendFields(event *zerolog.Event, fields map[string]any) {
	for k, v := range fields {
		// Type switch to avoid double JSON serialization of strings
		switch val := v.(type) {
		case error:
			event.Str(k, val.Error())
		case string:
			event.Str(k, val)
		case int:
			event.Int(k, val)
		case int64:
			event.Int64(k, val)
		case float64:
			event.Float64(k, val)
		case bool:
			event.Bool(k, val)
		default:
			event.Interface(k, v) // Fallback for struct, slice and maps
		}
	}
}

func Debug(message string) {
	logMessage(DEBUG, "", message, nil)
}

func DebugC(component string, message string) {
	logMessage(DEBUG, component, message, nil)
}

func Debugf(message string, ss ...any) {
	logMessage(DEBUG, "", fmt.Sprintf(message, ss...), nil)
}

func DebugF(message string, fields map[string]any) {
	logMessage(DEBUG, "", message, fields)
}

func DebugCF(component string, message string, fields map[string]any) {
	logMessage(DEBUG, component, message, fields)
}

func Info(message string) {
	logMessage(INFO, "", message, nil)
}

func InfoC(component string, message string) {
	logMessage(INFO, component, message, nil)
}

func InfoF(message string, fields map[string]any) {
	logMessage(INFO, "", message, fields)
}

func Infof(message string, ss ...any) {
	logMessage(INFO, "", fmt.Sprintf(message, ss...), nil)
}

func InfoCF(component string, message string, fields map[string]any) {
	logMessage(INFO, component, message, fields)
}

func Warn(message string) {
	logMessage(WARN, "", message, nil)
}

func WarnC(component string, message string) {
	logMessage(WARN, component, message, nil)
}

func WarnF(message string, fields map[string]any) {
	logMessage(WARN, "", message, fields)
}

func WarnCF(component string, message string, fields map[string]any) {
	logMessage(WARN, component, message, fields)
}

func Warnf(message string, ss ...any) {
	logMessage(WARN, "", fmt.Sprintf(message, ss...), nil)
}

func Error(message string) {
	logMessage(ERROR, "", message, nil)
}

func ErrorC(component string, message string) {
	logMessage(ERROR, component, message, nil)
}

func Errorf(message string, ss ...any) {
	logMessage(ERROR, "", fmt.Sprintf(message, ss...), nil)
}

func ErrorF(message string, fields map[string]any) {
	logMessage(ERROR, "", message, fields)
}

func ErrorCF(component string, message string, fields map[string]any) {
	logMessage(ERROR, component, message, fields)
}

func Fatal(message string) {
	logMessage(FATAL, "", message, nil)
}

func FatalC(component string, message string) {
	logMessage(FATAL, component, message, nil)
}

func Fatalf(message string, ss ...any) {
	logMessage(FATAL, "", fmt.Sprintf(message, ss...), nil)
}

func FatalF(message string, fields map[string]any) {
	logMessage(FATAL, "", message, fields)
}

func FatalCF(component string, message string, fields map[string]any) {
	logMessage(FATAL, component, message, fields)
}

// traceFieldsFromContext returns trace correlation fields when the context
// carries a valid span, so exported log records can be linked to traces.
func traceFieldsFromContext(ctx context.Context) map[string]any {
	if ctx == nil {
		return nil
	}
	sc := oteltrace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return map[string]any{
		"trace_id": sc.TraceID().String(),
		"span_id":  sc.SpanID().String(),
	}
}

func logWithContext(level LogLevel, ctx context.Context, component, message string, fields map[string]any) {
	if traceFields := traceFieldsFromContext(ctx); len(traceFields) > 0 {
		if fields == nil {
			fields = traceFields
		} else {
			merged := make(map[string]any, len(fields)+2)
			maps.Copy(merged, fields)
			maps.Copy(merged, traceFields)
			fields = merged
		}
	}
	logMessage(level, component, message, fields)
}

func DebugCFCtx(ctx context.Context, component string, message string, fields map[string]any) {
	logWithContext(DEBUG, ctx, component, message, fields)
}

func InfoCFCtx(ctx context.Context, component string, message string, fields map[string]any) {
	logWithContext(INFO, ctx, component, message, fields)
}

func WarnCFCtx(ctx context.Context, component string, message string, fields map[string]any) {
	logWithContext(WARN, ctx, component, message, fields)
}

func ErrorCFCtx(ctx context.Context, component string, message string, fields map[string]any) {
	logWithContext(ERROR, ctx, component, message, fields)
}

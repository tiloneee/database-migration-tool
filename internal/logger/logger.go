package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

// Init initializes the global logger
func Init(level, format, outputPath string) error {
	var config zap.Config

	if format == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Parse log level
	parsedLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		return err
	}
	config.Level = zap.NewAtomicLevelAt(parsedLevel)

	// Set output path
	if outputPath == "stdout" || outputPath == "" {
		config.OutputPaths = []string{"stdout"}
	} else {
		config.OutputPaths = []string{outputPath}
	}

	config.ErrorOutputPaths = []string{"stderr"}

	// Build logger
	logger, err := config.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return err
	}

	Log = logger
	return nil
}

// Close flushes any buffered log entries
func Close() {
	if Log != nil {
		_ = Log.Sync()
	}
}

// Helper functions for common logging patterns

func Debug(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Debug(msg, fields...)
	}
}

func Info(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Info(msg, fields...)
	}
}

func Warn(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Warn(msg, fields...)
	}
}

func Error(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Error(msg, fields...)
	}
}

func Fatal(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Fatal(msg, fields...)
	}
	os.Exit(1)
}

// WithFields creates a logger with pre-set fields
func WithFields(fields ...zap.Field) *zap.Logger {
	if Log == nil {
		return zap.NewNop()
	}
	return Log.With(fields...)
}

// pkg/logging/logger.go
package logging

import (

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
	level zapcore.Level
}

type Config struct {
	Level            string   `json:"level" yaml:"level"`
	Development      bool     `json:"development" yaml:"development"`
	Encoding         string   `json:"encoding" yaml:"encoding"` // json or console
	OutputPaths      []string `json:"output_paths" yaml:"output_paths"`
	ErrorOutputPaths []string `json:"error_output_paths" yaml:"error_output_paths"`
}

func DefaultConfig() *Config {
	return &Config{
		Level:            "info",
		Development:      false,
		Encoding:         "json",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
}

func DevelopmentConfig() *Config {
	return &Config{
		Level:            "debug",
		Development:      true,
		Encoding:         "console",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
}

func NewLogger(config *Config) (*Logger, error) {
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		return nil, err
	}

	var zapConfig zap.Config
	if config.Development {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	zapConfig.Level = zap.NewAtomicLevelAt(level)
	
	// Set encoding - default to console for development, json for production
	if config.Encoding != "" {
		zapConfig.Encoding = config.Encoding
	} else if config.Development {
		zapConfig.Encoding = "console"
	} else {
		zapConfig.Encoding = "json"
	}
	
	zapConfig.OutputPaths = config.OutputPaths
	zapConfig.ErrorOutputPaths = config.ErrorOutputPaths

	// Customize encoder config
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapConfig.EncoderConfig.CallerKey = "caller"
	zapConfig.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	if config.Development {
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapConfig.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	}

	logger, err := zapConfig.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}

	return &Logger{
		Logger: logger,
		level:  level,
	}, nil
}

func NewSimpleLogger(debug bool) *Logger {
	var config *Config
	if debug {
		config = DevelopmentConfig()
	} else {
		config = DefaultConfig()
	}

	logger, err := NewLogger(config)
	if err != nil {
		// Fallback to a basic logger if configuration fails
		var fallback *zap.Logger
		if debug {
			fallback, _ = zap.NewDevelopment()
		} else {
			fallback, _ = zap.NewProduction()
		}
		return &Logger{Logger: fallback, level: zapcore.InfoLevel}
	}

	return logger
}

// Convenience methods
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With(zap.String("component", component)),
		level:  l.level,
	}
}

func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	return &Logger{
		Logger: l.Logger.With(fields...),
		level:  l.level,
	}
}

func (l *Logger) IsDebugEnabled() bool {
	return l.level <= zapcore.DebugLevel
}

func (l *Logger) IsInfoEnabled() bool {
	return l.level <= zapcore.InfoLevel
}

// Safe sync that doesn't panic
func (l *Logger) SafeSync() {
	if l.Logger != nil {
		_ = l.Logger.Sync()
	}
}

// Global logger instance
var globalLogger *Logger

func init() {
	globalLogger = NewSimpleLogger(false)
}

func SetGlobalLogger(logger *Logger) {
	if globalLogger != nil {
		globalLogger.SafeSync()
	}
	globalLogger = logger
}

func GetGlobalLogger() *Logger {
	return globalLogger
}

// Global logging functions
func Debug(msg string, fields ...zap.Field) {
	globalLogger.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	globalLogger.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	globalLogger.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	globalLogger.Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	globalLogger.Fatal(msg, fields...)
}

func Sync() {
	globalLogger.SafeSync()
}
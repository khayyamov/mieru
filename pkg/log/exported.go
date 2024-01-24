package log

import (
	"context"
	"io"
	"strings"
	"time"
)

var (
	// std is the name of the standard logger in stdlib `log`
	std        = New()
	StdDisable = true
)

func StandardLogger() *Logger {
	return std
}

// SetOutput sets the standard logger output.
func SetOutput(out io.Writer) {
	if StdDisable {

	} else {
		std.SetOutput(out)
	}
}

// SetFormatter sets the standard logger formatter.
func SetFormatter(formatter Formatter) {
	if StdDisable {

	} else {
		std.SetFormatter(formatter)
	}
}

// SetReportCaller sets whether the standard logger will include the calling
// method as a field.
func SetReportCaller(include bool) {
	if StdDisable {

	} else {
		std.SetReportCaller(include)
	}
}

// SetLevel sets the standard logger level.
func SetLevel(level string) {
	if StdDisable {

	} else {
		level = strings.ToUpper(level)
		switch level {
		case "FATAL":
			std.SetLevel(FatalLevel)
		case "ERROR":
			std.SetLevel(ErrorLevel)
		case "WARN":
			std.SetLevel(WarnLevel)
		case "INFO":
			std.SetLevel(InfoLevel)
		case "DEBUG":
			std.SetLevel(DebugLevel)
		case "TRACE":
			std.SetLevel(TraceLevel)
		default:
		}
	}
}

// GetLevel returns the standard logger level.
func GetLevel() Level {
	return std.GetLevel()
}

// IsLevelEnabled checks if the log level of the standard logger is greater than the level param
func IsLevelEnabled(level Level) bool {
	return std.IsLevelEnabled(level)
}

// WithError creates an entry from the standard logger and adds an error to it, using the value defined in ErrorKey as key.
func WithError(err error) *Entry {
	return std.WithField(ErrorKey, err)
}

// WithContext creates an entry from the standard logger and adds a context to it.
func WithContext(ctx context.Context) *Entry {
	return std.WithContext(ctx)
}

// WithField creates an entry from the standard logger and adds a field to
// it. If you want multiple fields, use `WithFields`.
//
// Note that it doesn't log until you call Debug, Print, Info, Warn, Fatal
// or Panic on the Entry it returns.
func WithField(key string, value interface{}) *Entry {
	return std.WithField(key, value)
}

// WithFields creates an entry from the standard logger and adds multiple
// fields to it. This is simply a helper for `WithField`, invoking it
// once for each field.
//
// Note that it doesn't log until you call Debug, Print, Info, Warn, Fatal
// or Panic on the Entry it returns.
func WithFields(fields Fields) *Entry {
	return std.WithFields(fields)
}

// WithTime creates an entry from the standard logger and overrides the time of
// logs generated with it.
//
// Note that it doesn't log until you call Debug, Print, Info, Warn, Fatal
// or Panic on the Entry it returns.
func WithTime(t time.Time) *Entry {
	return std.WithTime(t)
}

// Tracef logs a message at level Trace on the standard logger.
func Tracef(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Tracef(format, args...)
	}
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Debugf(format, args...)
	}
}

// Printf logs a message at level Info on the standard logger.
func Printf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Printf(format, args...)
	}
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Infof(format, args...)
	}
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Warnf(format, args...)
	}
}

// Warningf logs a message at level Warn on the standard logger.
func Warningf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Warningf(format, args...)
	}
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Errorf(format, args...)
	}
}

// Panicf logs a message at level Panic on the standard logger.
func Panicf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Panicf(format, args...)
	}
}

// Fatalf logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalf(format string, args ...interface{}) {
	if StdDisable {

	} else {
		std.Fatalf(format, args...)
	}
}

// Print logs a message at level Info on the standard logger.
func Print(args ...interface{}) {
	std.Print(args...)
}

// Panic logs a message at level Panic on the standard logger.
func Panic(args ...interface{}) {
	if StdDisable {

	} else {
		std.Panic(args...)
	}
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatal(args ...interface{}) {
	if StdDisable {

	} else {
		std.Fatal(args...)
	}
}

// Println logs a message at level Info on the standard logger.
func Println(args ...interface{}) {
	if StdDisable {

	} else {
		std.Println(args...)
	}
}

// Panicln logs a message at level Panic on the standard logger.
func Panicln(args ...interface{}) {
	if StdDisable {

	} else {
		std.Panicln(args...)
	}
}

// Fatalln logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalln(args ...interface{}) {
	if StdDisable {

	} else {
		std.Fatalln(args...)
	}
}

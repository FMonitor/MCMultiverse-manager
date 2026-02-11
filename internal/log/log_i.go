package log

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	LevelDebug = "DEBUG"
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
)

var Logger *zap.SugaredLogger

const (
	ansiReset  = "\u001b[0m"
	ansiRed    = "\u001b[38;2;200;0;0m"
	ansiRedHi  = "\u001b[38;2;255;80;80m"
	ansiGreen  = "\u001b[38;2;0;200;0m"
	ansiYellow = "\u001b[38;2;255;200;0m"
	ansiBlue   = "\u001b[38;2;80;160;255m"
	ansiPurple = "\u001b[38;2;170;85;255m"
	ansiCyan   = "\u001b[38;2;0;200;200m"
	ansiGray   = "\u001b[38;2;140;140;140m"
)

const (
	levelDebugColor = "\u001b[38;2;170;85;255m"
	levelInfoColor  = "\u001b[38;2;0;200;0m"
	levelWarnColor  = "\u001b[38;2;255;200;0m"
	levelErrorColor = "\u001b[38;2;255;80;80m"
	levelFatalColor = "\u001b[38;2;180;0;0m"
)

const componentFieldKey = "component"

var componentColorMap = map[string]string{
	"main":   ansiCyan,
	"db":     ansiBlue,
	"worker": ansiYellow,
	"test":   ansiPurple,
}

// SetupLogger Default: INFO
func SetupLogger(logLevel string) {
	// set log level
	var level zapcore.Level
	switch strings.ToUpper(logLevel) {
	case LevelDebug:
		level = zap.DebugLevel
	case LevelInfo:
		level = zap.InfoLevel
	case LevelWarn:
		level = zap.WarnLevel
	case LevelError:
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.ISO8601TimeEncoder, // UTC timezone
		EncodeLevel:    colorizeLevelEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
		zap.NewAtomicLevelAt(level),
	)

	Logger = zap.New(newComponentCore(core), zap.AddCaller()).Sugar()
}

type componentCore struct {
	core   zapcore.Core
	fields []zapcore.Field
}

func newComponentCore(core zapcore.Core) zapcore.Core {
	return &componentCore{core: core, fields: nil}
}

func (c *componentCore) Enabled(level zapcore.Level) bool {
	return c.core.Enabled(level)
}

func (c *componentCore) With(fields []zapcore.Field) zapcore.Core {
	if len(fields) == 0 {
		return c
	}

	newFields := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	newFields = append(newFields, c.fields...)
	newFields = append(newFields, fields...)

	return &componentCore{
		core:   c.core,
		fields: newFields,
	}
}

func (c *componentCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if !c.Enabled(entry.Level) {
		return ce
	}

	return ce.AddCore(entry, c)
}

func (c *componentCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	allFields := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	allFields = append(allFields, c.fields...)
	allFields = append(allFields, fields...)

	component := extractComponent(allFields)
	if component != "" {
		entry.Message = "[" + component + "] " + entry.Message
	}

	if color := messageColorForLevel(entry.Level); color != "" {
		entry.Message = colorize(color, entry.Message)
	} else if component != "" {
		if color, ok := componentColorMap[component]; ok {
			entry.Message = colorize(color, entry.Message)
		}
	}

	return c.core.Write(entry, removeComponentFields(allFields))
}

func (c *componentCore) Sync() error {
	return c.core.Sync()
}

func extractComponent(fields []zapcore.Field) string {
	for _, field := range fields {
		if field.Key != componentFieldKey {
			continue
		}
		switch field.Type {
		case zapcore.StringType:
			return field.String
		case zapcore.StringerType:
			if field.Interface != nil {
				if stringer, ok := field.Interface.(interface{ String() string }); ok {
					return stringer.String()
				}
			}
		}
	}

	return ""
}

func removeComponentFields(fields []zapcore.Field) []zapcore.Field {
	if len(fields) == 0 {
		return fields
	}

	filtered := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		if field.Key == componentFieldKey {
			continue
		}
		filtered = append(filtered, field)
	}

	return filtered
}

func colorize(color string, text string) string {
	if color == "" {
		return text
	}

	return color + text + ansiReset
}

func colorizeLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var color string
	switch level {
	case zapcore.DebugLevel:
		color = levelDebugColor
	case zapcore.InfoLevel:
		color = levelInfoColor
	case zapcore.WarnLevel:
		color = levelWarnColor
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel:
		color = levelErrorColor
	case zapcore.FatalLevel:
		color = levelFatalColor
	default:
		color = ansiReset
	}

	enc.AppendString(colorize(color, level.CapitalString()))
}

func messageColorForLevel(level zapcore.Level) string {
	switch level {
	case zapcore.WarnLevel:
		return levelWarnColor
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel:
		return levelErrorColor
	case zapcore.FatalLevel:
		return levelFatalColor
	default:
		return ""
	}
}

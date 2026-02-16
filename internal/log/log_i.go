package log

import (
	"os"
	"strings"
	"time"

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
	ansiOrange = "\u001b[38;2;255;140;0m"
	ansiGreen  = "\u001b[38;2;0;200;0m"
	ansiLime   = "\u001b[38;2;160;220;80m"
	ansiYellow = "\u001b[38;2;255;200;0m"
	ansiBlue   = "\u001b[38;2;80;160;255m"
	ansiTeal   = "\u001b[38;2;0;170;160m"
	ansiPurple = "\u001b[38;2;170;85;255m"
	ansiPink   = "\u001b[38;2;255;120;210m"
	ansiCyan   = "\u001b[38;2;0;200;200m"
	ansiGray   = "\u001b[38;2;140;140;140m"
	ansiWhite  = "\u001b[38;2;220;220;220m"
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
	"main":        ansiBlue,
	"worker":      ansiOrange,
	"test":        ansiGray,
	"config":      ansiPink,
	"pgsql":       ansiCyan,
	"servertap":   ansiLime,
	"cmdreceiver": ansiTeal,
	"webservice":  ansiPink,
}

var colorPresetMap = map[string]string{
	"orange": ansiOrange,
	"lime":   ansiLime,
	"cyan":   ansiCyan,
	"teal":   ansiTeal,
	"blue":   ansiBlue,
	"pink":   ansiPink,
	"gray":   ansiGray,
	"white":  ansiWhite,
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
		TimeKey:          "time",
		LevelKey:         "level",
		NameKey:          "logger",
		CallerKey:        "",
		MessageKey:       "msg",
		StacktraceKey:    "stacktrace",
		LineEnding:       zapcore.DefaultLineEnding,
		ConsoleSeparator: " ",
		EncodeTime:       shortISO8601TimeEncoder,
		EncodeLevel:      colorizeLevelEncoder,
		EncodeDuration:   zapcore.SecondsDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
		zap.NewAtomicLevelAt(level),
	)

	Logger = zap.New(newComponentCore(core), zap.AddCaller()).Sugar()
}

func Component(name string) *zap.SugaredLogger {
	if Logger == nil {
		SetupLogger(LevelInfo)
	}
	return Logger.With("component", name)
}

// RegisterComponentColor sets one component color using a preset name, e.g. "repo", "teal".
func RegisterComponentColor(component string, preset string) bool {
	color, ok := colorPresetMap[strings.ToLower(strings.TrimSpace(preset))]
	if !ok || component == "" {
		return false
	}
	componentColorMap[component] = color
	return true
}

// RegisterComponentPalette sets multiple component colors at once.
func RegisterComponentPalette(palette map[string]string) {
	for component, preset := range palette {
		_ = RegisterComponentColor(component, preset)
	}
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

	msg := entry.Message
	component := extractComponent(allFields)
	if component != "" {
		msg = "[" + component + "] " + msg
	}

	if color := messageColorForLevel(entry.Level); color != "" {
		msg = colorize(color, msg)
	} else if component != "" {
		if color, ok := componentColorMap[component]; ok {
			msg = colorize(color, msg)
		}
	}

	if entry.Level >= zapcore.WarnLevel && entry.Caller.Defined {
		msg += "\t" + colorize(ansiGray, entry.Caller.TrimmedPath())
	}

	entry.Message = msg
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

func shortISO8601TimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02T15:04:05.000"))
}

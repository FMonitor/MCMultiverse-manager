package log

import "testing"

func TestSetupLogger(t *testing.T) {
	SetupLogger(LevelDebug)
	Logger = Logger.With("component", "test")
	// debug
	Logger.Debug("This is debug level log")
	Logger.Debugf("This is debug level log: %s", "test")
	// warn
	Logger.Warn("This is warn level log")
	Logger.Warnf("This is warn level log: %s", "test")
	// info
	Logger.Info("This is info level log")
	Logger.Infof("This is info level log: %s", "test")
	// error
	Logger.Error("This is error level log")
	Logger.Errorf("This is error level log: %s", "test")
}

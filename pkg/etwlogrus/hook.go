package etwlogrus

import (
	"fmt"
	"reflect"

	"github.com/Microsoft/go-winio/internal/etw"
	"github.com/sirupsen/logrus"
)

// Hook is a Logrus hook which logs received events to ETW.
type Hook struct {
	provider *etw.Provider
}

// NewHook registers a new ETW provider and returns a hook to log from it.
func NewHook(providerName string) (*Hook, error) {
	hook := Hook{}

	provider, err := etw.NewProvider(providerName, nil)
	if err != nil {
		return nil, err
	}
	hook.provider = provider

	return &hook, nil
}

// Levels returns the set of levels that this hook wants to receive log entries
// for.
func (h *Hook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.TraceLevel,
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}

// Fire receives each Logrus entry as it is logged, and logs it to ETW.
func (h *Hook) Fire(e *logrus.Entry) error {
	level := etw.Level(e.Level)
	if !h.provider.IsEnabledForLevel(level) {
		return nil
	}

	// Reserve extra space for the message field.
	fields := make([]etw.FieldOpt, 0, len(e.Data)+1)

	fields = append(fields, etw.StringField("Message", e.Message))

	for k, v := range e.Data {
		switch v := v.(type) {
		case string:
			fields = append(fields, etw.StringField(k, v))
		default:
			fields = append(fields, etw.StringField(k, fmt.Sprintf("<unknown type: %v> %v", reflect.TypeOf(v), v)))
		}
	}

	// We could try to map Logrus levels to ETW levels, but we would lose some
	// fidelity as there are fewer ETW levels. So instead we use the level
	// directly.
	return h.provider.WriteEvent(
		"LogrusEntry",
		etw.WithEventOpts(etw.WithLevel(level)),
		fields)
}

// Close cleans up the hook and closes the ETW provider.
func (h *Hook) Close() error {
	return h.provider.Close()
}

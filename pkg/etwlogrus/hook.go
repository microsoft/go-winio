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
	if !h.provider.IsEnabledForLevel(etw.Level(e.Level)) {
		return nil
	}

	opts := make([]interface{}, len(e.Data))
	i := 0

	// We could try to map Logrus levels to ETW levels, but we would lose some
	// fidelity as there are fewer ETW levels. So instead we use the level
	// directly.
	opts[i] = etw.WithLevel(etw.Level(e.Level))
	i++

	opts[i] = etw.StringField("Message", e.Message)
	i++

	for k, v := range e.Data {
		switch v := v.(type) {
		case string:
			opts[i] = etw.StringField(k, v)
		default:
			opts[i] = etw.StringField(k, fmt.Sprintf("<unknown type: %v> %v", reflect.TypeOf(v), v))
		}
		i++
	}

	h.provider.WriteEvent(
		"LogrusEntry",
		opts...)

	return nil
}

// Close cleans up the hook and closes the ETW provider.
func (h *Hook) Close() error {
	return h.provider.Close()
}

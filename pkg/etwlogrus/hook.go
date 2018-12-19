package etwlogrus

import (
	"fmt"
	"reflect"

	"github.com/Microsoft/go-winio/internal/etw"
	"github.com/sirupsen/logrus"

	"golang.org/x/sys/windows"
)

// Hook is a Logrus hook which logs received events to ETW.
type Hook struct {
	provider *etw.Provider
}

// NewHook registers a new ETW provider and returns a hook to log from it.
func NewHook(providerName string, providerID *windows.GUID) (*Hook, error) {
	hook := Hook{}

	provider, err := etw.NewProvider(providerName, providerID, nil)
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

	descriptor := etw.NewEventDescriptor()

	// We could try to map Logrus levels to ETW levels, but we would lose some
	// fidelity as there are fewer ETW levels. So instead we use the level
	// directly.
	descriptor.Level = etw.Level(e.Level)

	event := etw.NewEvent("LogrusEntry", descriptor)

	event.Metadata.AddField("Message", etw.InTypeAnsiString)
	event.Data.AddString(e.Message)

	for k, v := range e.Data {
		switch v := v.(type) {
		case string:
			event.Metadata.AddField(k, etw.InTypeAnsiString)
			event.Data.AddString(v)
		default:
			event.Metadata.AddField(k, etw.InTypeAnsiString)
			event.Data.AddString(fmt.Sprintf("<unknown type: %v> %v", reflect.TypeOf(v), v))
		}
	}

	h.provider.WriteEvent(event)

	return nil
}

// Close cleans up the hook and closes the ETW provider.
func (h *Hook) Close() error {
	return h.provider.Close()
}

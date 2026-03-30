//go:build windows

package etwslog

import (
	"context"
	"errors"
	"log/slog"
	"sort"

	"github.com/Microsoft/go-winio/pkg/etw"
)

const defaultEventName = "SlogEntry"

// ErrNoProvider is returned when a handler is created without a provider being configured.
var ErrNoProvider = errors.New("no ETW registered provider")

// Handler is a [slog.Handler] which logs received events to ETW.
type Handler struct {
	provider      *etw.Provider
	closeProvider bool
	// allows setting the event name
	getName func(slog.Record) string
	// returns additional options to add to the event
	getEventsOpts func(slog.Record) []etw.EventOpt
	// pre-computed attrs from WithAttrs
	attrs []slog.Attr
	// group prefix from WithGroup
	group string
}

// slogLevelToETWLevel maps slog levels to ETW levels using range-based mapping.
func slogLevelToETWLevel(level slog.Level) etw.Level {
	switch {
	case level >= slog.LevelError:
		return etw.LevelError
	case level >= slog.LevelWarn:
		return etw.LevelWarning
	case level >= slog.LevelInfo:
		return etw.LevelInfo
	default:
		return etw.LevelVerbose
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return h.provider.IsEnabledForLevel(slogLevelToETWLevel(level))
}

// Handle writes the record to ETW.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	level := slogLevelToETWLevel(r.Level)
	if !h.provider.IsEnabledForLevel(level) {
		return nil
	}

	name := defaultEventName
	if h.getName != nil {
		if n := h.getName(r); n != "" {
			name = n
		}
	}

	// extra room for two more options in addition to log level to avoid repeated reallocations
	// if the user also provides options
	opts := make([]etw.EventOpt, 0, 3)
	opts = append(opts, etw.WithLevel(level))
	if h.getEventsOpts != nil {
		opts = append(opts, h.getEventsOpts(r)...)
	}

	// Collect all attrs: pre-computed from WithAttrs + record attrs.
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// Sort the attrs by key so they are consistent in each instance
	// of an event. Otherwise, the fields don't line up in WPA.
	// Put "error" last because it is optional in some events.
	var errorAttr *slog.Attr
	sorted := make([]slog.Attr, 0, len(allAttrs))
	for i := range allAttrs {
		if allAttrs[i].Key == "error" {
			a := allAttrs[i]
			errorAttr = &a
		} else {
			sorted = append(sorted, allAttrs[i])
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})

	// Reserve extra space for the message and time fields.
	fields := make([]etw.FieldOpt, 0, len(allAttrs)+2)
	fields = append(fields, etw.StringField("Message", r.Message))
	fields = append(fields, etw.Time("Time", r.Time))
	for _, a := range sorted {
		fields = append(fields, attrToETWField(h.group, a))
	}
	if errorAttr != nil {
		fields = append(fields, attrToETWField(h.group, *errorAttr))
	}

	// Firing an ETW event is essentially best effort, as the event write can
	// fail for reasons completely out of the control of the event writer (such
	// as a session listening for the event having no available space in its
	// buffers). Therefore, we don't return the error from WriteEvent, as it is
	// just noise in many cases.
	_ = h.provider.WriteEvent(name, opts, fields)

	return nil
}

// WithAttrs returns a new Handler with the given attributes pre-computed.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &Handler{
		provider:      h.provider,
		closeProvider: h.closeProvider,
		getName:       h.getName,
		getEventsOpts: h.getEventsOpts,
		attrs:         newAttrs,
		group:         h.group,
	}
}

// WithGroup returns a new Handler with the given group name prepended to all
// attribute keys.
func (h *Handler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &Handler{
		provider:      h.provider,
		closeProvider: h.closeProvider,
		getName:       h.getName,
		getEventsOpts: h.getEventsOpts,
		attrs:         h.attrs,
		group:         newGroup,
	}
}

// Close cleans up the handler and closes the ETW provider. If the provider was
// registered by etwslog, it will be closed as part of Close. If the
// provider was passed in, it will not be closed.
func (h *Handler) Close() error {
	if h.closeProvider {
		return h.provider.Close()
	}
	return nil
}

func (h *Handler) validate() error {
	if h.provider == nil {
		return ErrNoProvider
	}
	return nil
}

// attrToETWField converts a slog.Attr to an etw.FieldOpt.
func attrToETWField(group string, a slog.Attr) etw.FieldOpt {
	key := a.Key
	if group != "" {
		key = group + "." + key
	}

	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindBool:
		return etw.BoolField(key, v.Bool())
	case slog.KindInt64:
		return etw.Int64Field(key, v.Int64())
	case slog.KindUint64:
		return etw.Uint64Field(key, v.Uint64())
	case slog.KindFloat64:
		return etw.Float64Field(key, v.Float64())
	case slog.KindString:
		return etw.StringField(key, v.String())
	case slog.KindTime:
		return etw.Time(key, v.Time())
	case slog.KindGroup:
		attrs := v.Group()
		fields := make([]etw.FieldOpt, 0, len(attrs))
		for _, ga := range attrs {
			fields = append(fields, attrToETWField("", ga))
		}
		return etw.Struct(key, fields...)
	default:
		// KindAny and anything else
		return etw.SmartField(key, v.Any())
	}
}

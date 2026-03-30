//go:build windows

package etwslog

import (
	"log/slog"

	"github.com/Microsoft/go-winio/pkg/etw"
)

// HandlerOpt is an option to change the behavior of the slog ETW handler.
type HandlerOpt func(*Handler) error

// NewHandler registers a new ETW provider and returns a handler to log from it.
// The provider will be closed when the handler is closed.
func NewHandler(providerName string, opts ...HandlerOpt) (*Handler, error) {
	opts = append(opts, WithNewETWProvider(providerName))

	return NewHandlerFromOpts(opts...)
}

// NewHandlerFromProvider creates a new handler based on an existing ETW provider.
// The provider will not be closed when the handler is closed.
func NewHandlerFromProvider(provider *etw.Provider, opts ...HandlerOpt) (*Handler, error) {
	opts = append(opts, WithExistingETWProvider(provider))

	return NewHandlerFromOpts(opts...)
}

// NewHandlerFromOpts creates a new handler with the provided options.
// An error is returned if the handler does not have a valid provider.
func NewHandlerFromOpts(opts ...HandlerOpt) (*Handler, error) {
	h := &Handler{}

	for _, o := range opts {
		if err := o(h); err != nil {
			return nil, err
		}
	}
	return h, h.validate()
}

// WithNewETWProvider registers a new ETW provider and sets the handler to log using it.
// The provider will be closed when the handler is closed.
func WithNewETWProvider(n string) HandlerOpt {
	return func(h *Handler) error {
		provider, err := etw.NewProvider(n, nil)
		if err != nil {
			return err
		}

		h.provider = provider
		h.closeProvider = true
		return nil
	}
}

// WithExistingETWProvider configures the handler to use an existing ETW provider.
// The provider will not be closed when the handler is closed.
func WithExistingETWProvider(p *etw.Provider) HandlerOpt {
	return func(h *Handler) error {
		h.provider = p
		h.closeProvider = false
		return nil
	}
}

// WithGetName sets the ETW EventName of an event to the value returned by f.
// If the name is empty, the default event name will be used.
func WithGetName(f func(slog.Record) string) HandlerOpt {
	return func(h *Handler) error {
		h.getName = f
		return nil
	}
}

// WithEventOpts allows additional ETW event properties (keywords, tags, etc.) to be specified.
func WithEventOpts(f func(slog.Record) []etw.EventOpt) HandlerOpt {
	return func(h *Handler) error {
		h.getEventsOpts = f
		return nil
	}
}

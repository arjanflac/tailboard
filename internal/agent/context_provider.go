package agent

import "github.com/arjanflac/tailboard/internal/privacy"

type contextProvider interface {
	CurrentContext() (privacy.Context, error)
}

type contextProviderReporter interface {
	Layer() string
}

type noopContextProvider struct{}

func (noopContextProvider) CurrentContext() (privacy.Context, error) {
	return privacy.Context{}, nil
}

func (noopContextProvider) Layer() string { return "unavailable" }

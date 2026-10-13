package config

import (
	"errors"
	"fmt"
)

// Environment variable names understood by Equus.
const (
	EnvHome          = "EQUUS_HOME"
	EnvAPIKey        = "EQUUS_API_KEY"
	EnvBaseURL       = "EQUUS_BASE_URL"
	EnvModel         = "EQUUS_MODEL"
	EnvModelProvider = "EQUUS_MODEL_PROVIDER"
)

var (
	errNoProviderSelected = errors.New("no model provider selected: set model_provider or define exactly one provider")
)

func errProviderUnknown(name string) error {
	return fmt.Errorf("model provider %q is not defined in model_providers", name)
}

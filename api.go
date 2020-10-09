// +build !js

package webrtc

import (
	"github.com/pion/logging"
)

// API bundles the global funcions of the WebRTC and ORTC API.
// Some of these functions are also exported globally using the
// defaultAPI object. Note that the global version of the API
// may be phased out in the future.
type API struct {
	settingEngine *SettingEngine
}

// NewAPI Creates a new API object for keeping semi-global settings to WebRTC objects
func NewAPI(options ...func(*API)) *API {
	a := &API{}

	for _, o := range options {
		o(a)
	}

	if a.settingEngine == nil {
		a.settingEngine = &SettingEngine{}
	}

	if a.settingEngine.LoggerFactory == nil {
		a.settingEngine.LoggerFactory = logging.NewDefaultLoggerFactory()
	}



	return a
}

// WithSettingEngine allows providing a SettingEngine to the API.
// Settings should not be changed after passing the engine to an API.
func WithSettingEngine(s SettingEngine) func(a *API) {
	return func(a *API) {
		a.settingEngine = &s
	}
}

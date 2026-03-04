package pluginbridge

import (
	"fmt"
	"net/http"
)

// pluginAPIRoundTripper adapts a PluginAPI to the http.RoundTripper interface.
type pluginAPIRoundTripper struct {
	api PluginAPI
}

func (t *pluginAPIRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := t.api.PluginHTTP(req)
	if resp == nil {
		return nil, fmt.Errorf("PluginHTTP returned nil response for %s %s", req.Method, req.URL.Path)
	}
	return resp, nil
}

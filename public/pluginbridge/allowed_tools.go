package pluginbridge

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost-plugin-agents/public/bridgeclient"
)

// AllowedToolsList accepts both the current object form and legacy string arrays.
type AllowedToolsList []bridgeclient.AllowedToolRef

// UnmarshalJSON keeps backwards compatibility with legacy string-only tool lists.
func (l *AllowedToolsList) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*l = nil
		return nil
	}

	var refs []bridgeclient.AllowedToolRef
	if err := json.Unmarshal(data, &refs); err == nil {
		*l = AllowedToolsList(refs)
		return nil
	}

	var legacy []string
	if err := json.Unmarshal(data, &legacy); err == nil {
		refs = make([]bridgeclient.AllowedToolRef, 0, len(legacy))
		for _, name := range legacy {
			refs = append(refs, bridgeclient.AllowedToolRef{Name: name})
		}
		*l = AllowedToolsList(refs)
		return nil
	}

	return fmt.Errorf("allowed_tools must be an array of tool refs or tool names")
}

package action

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// renderTemplate renders a Go text/template string with the given AutomationContext.
func renderTemplate(tmplStr string, ctx *model.AutomationContext) (string, error) {
	tmpl, err := template.New("action").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

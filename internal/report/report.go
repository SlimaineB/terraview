package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"terraview/internal/graph"
)

const embeddedDataMarker = `<script id="terraview-embedded-data" type="application/json" style="display:none;"></script>`

func Write(path string, data *graph.Data, template []byte) error {
	graphJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal graph data: %w", err)
	}

	embedded := fmt.Sprintf(`<script id="terraview-embedded-data" type="application/json" style="display:none;">%s</script>`, string(graphJSON))
	html := bytes.Replace(template, []byte(embeddedDataMarker), []byte(embedded), 1)

	if err := os.WriteFile(path, html, 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

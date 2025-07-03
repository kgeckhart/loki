package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/v3/integration/client"
)

func TestCanAddMetadata(t *testing.T) {
	tenantID := "test-tenant-id"
	c := client.New(tenantID, "", "http://localhost:3100")
	require.NotNil(t, c)

	now := time.Now()
	err := c.PushLogLine("test message", time.Now(), nil)
	require.NoError(t, err)

	resp, err := c.RunRangeQueryWithStartEnd(t.Context(), `{job=~".+"} |= "test message"`, now, time.Now())
	require.NoError(t, err)
	require.NotNil(t, resp)

	respJson, err := json.MarshalIndent(resp, "", "  ")
	t.Logf("Response: %v", string(respJson))
}

const jsonLogFmt = `
{
	"meta": {
		"namespace": "%s",
		"requestId": "%d",
		"req": {
			"url": "/inventory?productIds=1234,5678&storeIds=1,2",
			"route": "%s"
		}
	},
	"1234": {
		"products": {
			"5678": {
				"quantity": 0,
				"status": "OutOfStock"
			}
		}
	},
	"message": "%s"
}
`

func TestCanAddLabelsAndMetadataFromJson(t *testing.T) {
	tenantID := "test-tenant-id-2"
	c := client.New(tenantID, "", "http://localhost:3100")
	require.NotNil(t, c)

	now := time.Now()
	// Compact the JSON to remove whitespace
	var compactedJson bytes.Buffer
	err := json.Compact(&compactedJson, []byte(fmt.Sprintf(jsonLogFmt, "test-namespace", now.UnixNano(), "/inventory", "out_of_stock")))
	require.NoError(t, err)

	extraLabels := map[string]string{
		"service_name": "aaa-bbb-ccc",
		"env":          "prod",
	}

	err = c.PushLogLine(compactedJson.String(), time.Now(), nil, extraLabels)
	require.NoError(t, err)

	resp, err := c.RunRangeQueryWithStartEnd(t.Context(), `{job=~".+"} |= "out_of_stock"`, now, time.Now())
	require.NoError(t, err)
	require.NotNil(t, resp)

	respJson, err := json.MarshalIndent(resp, "", "  ")
	t.Logf("Response: %v", string(respJson))
}

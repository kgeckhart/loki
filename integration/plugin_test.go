package integration

import (
	"encoding/json"
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
	err := c.PushLogLine("test message", time.Now(), nil, nil)
	require.NoError(t, err)

	resp, err := c.RunRangeQueryWithStartEnd(t.Context(), `{job=~".+"} |= "test message"`, now, time.Now())
	require.NoError(t, err)
	require.NotNil(t, resp)

	respJson, err := json.Marshal(resp)
	t.Logf("Response: %v", string(respJson))
}

func TestCanAddMetadataFromJson(t *testing.T) {
	tenantID := "test-tenant-id-2"
	c := client.New(tenantID, "", "http://localhost:3100")
	require.NotNil(t, c)

	now := time.Now()
	jsonLog := `{"meta":{"namespace":"test-namespace","requestId":"12345","req":{"url":"http://example.com"}},"message":"test message"}`
	err := c.PushLogLine(jsonLog, time.Now(), nil, nil)
	require.NoError(t, err)

	resp, err := c.RunRangeQueryWithStartEnd(t.Context(), `{job=~".+"} |= "test message"`, now, time.Now())
	require.NoError(t, err)
	require.NotNil(t, resp)

	respJson, err := json.Marshal(resp)
	t.Logf("Response: %v", string(respJson))
}

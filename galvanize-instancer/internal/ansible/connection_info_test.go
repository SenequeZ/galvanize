package ansible

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePublishedPorts_ExtractsOptionalProtocolHints(t *testing.T) {
	params := map[string]interface{}{
		"image": "example:latest",
		"published_ports": []interface{}{
			"22/ssh",
			"8080:80/http",
			"53/udp",
			443,
		},
	}

	normalized, hints := normalizePublishedPorts(params)

	assert.Equal(t, "example:latest", normalized["image"])

	ports, ok := normalized["published_ports"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{"22", "8080:80", "53/udp", 443}, ports)

	assert.Equal(t, "ssh", hints[22])
	assert.Equal(t, "http", hints[80])
	_, hasUDPHint := hints[53]
	assert.False(t, hasUDPHint)
	_, has443Hint := hints[443]
	assert.False(t, has443Hint)
}

func TestGetConnectionInfo_UsesHintedProtocolWhenProvided(t *testing.T) {
	containers := []ContainerInfo{
		{
			Publishers: []PublisherInfo{
				{Protocol: "tcp", PublishedPort: 40022, TargetPort: 22, URL: "0.0.0.0"},
				{Protocol: "tcp", PublishedPort: 40080, TargetPort: 80, URL: "0.0.0.0"},
			},
		},
	}

	conn, err := GetConnectionInfo(containers, "instancer.example.com", map[int]string{22: "ssh"})
	require.NoError(t, err)
	assert.Equal(t, "ssh://instancer.example.com:40022\ntcp://instancer.example.com:40080", conn)
}

func TestGetConnectionInfo_UsesDockerProtocolWhenNoHint(t *testing.T) {
	containers := []ContainerInfo{
		{
			Publishers: []PublisherInfo{
				{Protocol: "tcp", PublishedPort: 40022, TargetPort: 22, URL: "0.0.0.0"},
			},
		},
	}

	conn, err := GetConnectionInfo(containers, "instancer.example.com", nil)
	require.NoError(t, err)
	assert.Equal(t, "tcp://instancer.example.com:40022", conn)
}

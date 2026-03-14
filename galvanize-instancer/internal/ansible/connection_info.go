package ansible

import (
	"fmt"
	"strings"
)

func GetConnectionInfo(containerInfos []ContainerInfo, host string, protocolHints map[int]string) (string, error) {
	traefikLabel := ""
	for _, ci := range containerInfos {
		for labelKey, labelValue := range ci.Labels {
			if strings.HasPrefix(labelKey, "traefik.http.routers.") {
				traefikLabel = labelValue
				break
			}
		}
		// Check if we have a traefik label
		if traefikLabel != "" {
			labelParts := strings.Split(traefikLabel, "`")
			if len(labelParts) >= 2 {
				domainName := "https://" + labelParts[1] + "/"
				return domainName, nil
			}
		}
		// Check if we have published ports
		var ports []string
		for _, pub := range ci.Publishers {
			if pub.PublishedPort != 0 {
				// If IP is IPv6, continue
				if strings.Contains(pub.URL, ":") {
					continue
				}
				scheme := pub.Protocol
				if protocolHints != nil {
					if hintedScheme, ok := protocolHints[pub.TargetPort]; ok {
						scheme = hintedScheme
					}
				}
				ports = append(ports, fmt.Sprintf("%s://%s:%d", scheme, host, pub.PublishedPort))
			}
		}
		if len(ports) > 0 {
			return strings.Join(ports, "\n"), nil
		}
	}
	return "", fmt.Errorf("no connection info found")
}

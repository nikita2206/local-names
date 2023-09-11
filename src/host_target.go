package src

import "net"

// Contains code pertaining to managing the targets (containers or services that are running directly on host).

// HostTarget Represents one of the possible targets of the Resolver + Load Balancer: either a DockerContainer, or a manually configured IP:port address.
type HostTarget struct {
	DockerContainer  DockerContainer
	ConfiguredTarget net.IP
	ConfiguredPort   uint16
}

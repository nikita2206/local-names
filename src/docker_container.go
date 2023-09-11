package src

// These intentionally don't match the assigned protocol numbers, because those do not specify HTTP.
const (
	ProxyProtoHttp = 1
	ProxyProtoTcp  = 2
	ProxyProtoUdp  = 3
)

type DockerContainer struct {
	Id    string
	Names []string
	// PID of the container's main process.
	Pid int

	// Reverse-proxied targets: main target, and optionally additional targets.
	ProxiedPort            DockerPort
	AdditionalProxiedPorts []DockerPort
}

type DockerPort struct {
	// In-container port.
	Port uint16
	// If container has multiple ports (AdditionalProxiedPorts), it's possible to proxy different subdomains to different ports.
	Subdomain string
	Protocol  int
}

package src

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

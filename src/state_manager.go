package src

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/samber/lo"
	"net"
	"strings"
	"sync"
)

// These intentionally don't match the official assigned protocol numbers, because those do not specify HTTP.
const (
	// ProxyTypeHttp Proxying the given endpoint using HTTP protocol
	ProxyTypeHttp  = 1
	ProxyTypeTcp   = 2
	ProxyTypeUdp   = 3
	ProxyTypeHttps = 4

	// ProxyDisabledOnlyDns No proxying, only resolve the endpoint IP
	ProxyDisabledOnlyDns = 5
)

var InferredHttpPorts = []uint16{80, 8080, 8000, 8010, 443, 3000, 15672}

const (
	EndpointHostType      = 1
	EndpointContainerType = 2
)

type HostEndpoint struct {
	Ip   net.IP
	Port uint16
	Pid  int

	// InjectedIntoDockerNetworks is a list of Docker network IDs, where this endpoint is injected.
	// If you override a docker container with a host endpoint, then that host endpoint will
	//   have this list populated with the networks to which that container belonged.
	// It's also possible that you don't override a docker container, but still want to inject
	//   a host endpoint into a docker network, in this case this variable will also be populated.
	InjectedIntoDockerNetworks []string
}

type ContainerEndpoint struct {
	ContainerPort uint16
	ContainerName string

	// OverridenBy refers to the HostEndpoint that overrides this one, making it possible to replace a container with locally-running service.
	OverridenBy *Endpoint

	// DockerContainer is the container ID that this endpoint belongs to.
	DockerContainer string
	// DockerContainerName is the container name that this endpoint belongs to, this allows to restore the relation to the container if it's removed and recreated.
	DockerContainerName string
}

type Endpoint struct {
	Id                uint64
	Name              string
	Aliases           []string
	EndpointType      int // EndpointHostType or EndpointContainerType
	HostEndpoint      *HostEndpoint
	ContainerEndpoint *ContainerEndpoint

	// ManualOverride is true if this endpoint was force-saved or some manual changes were made to it, this is true.
	// This allows a user to ensure that some specific host-endpoint is always proxied, even if it's not currently running; or that name is changed and that change persists.
	ManualOverride bool

	// ProxyType whether this endpoint is proxied for the host, and which protocol is used.
	ProxyType int
}

type StateHandle struct {
	DefaultTld string
	IdCounter  uint64
	Endpoints  []Endpoint
}

var _state = StateHandle{}
var _stateMutationLock = sync.RWMutex{}

// Initialize state from static configuration: this can declare static endpoints.
func initFromStaticConfig() {

}

// InitFromDockerState Initialize state from Docker state: this can declare endpoints for all running containers.
// Each container declares EXPOSED ports, these become endpoints and are proxied from the host.
func (s *StateHandle) InitFromDockerState(containers []types.Container, networks []types.NetworkResource) {

	networksMap := lo.KeyBy(networks, func(n types.NetworkResource) string { return n.ID })

	for _, cnt := range containers {
		println("Container:", cnt.ID, "with name", cnt.Names[0], "and labels", cnt.Labels)

		var exposedHttpPort uint16
		for _, port := range cnt.Ports {
			if port.Type == "tcp" && lo.Contains(InferredHttpPorts, port.PrivatePort) {
				exposedHttpPort = port.PrivatePort
				break
			}
		}

		proposedNames := make([]string, len(cnt.Names)*5)

		for _, name := range cnt.Names {
			normalizedName := NormalizeName(name)
			proposedNames = append(proposedNames, normalizedName+"."+s.DefaultTld)

			// If the name ends with "-1", which always happens with Docker Compose
			if strings.LastIndex(normalizedName, "-1") == len(normalizedName)-2 {
				normalizedName = normalizedName[:len(normalizedName)-2]
				proposedNames = append(proposedNames, normalizedName+"."+s.DefaultTld)
			}
		}

		composeProject, okProject := cnt.Labels["com.docker.compose.project"]
		serviceName, okService := cnt.Labels["com.docker.compose.service"]
		if okProject && okService {
			proposedNames = append(proposedNames, NormalizeName(serviceName+"."+composeProject)+"."+s.DefaultTld)
		}

		cntNetworkKey, cntBridgeNetworkFound := lo.FindKeyBy(cnt.NetworkSettings.Networks, func(k string, v *network.EndpointSettings) bool {
			cntNetwork, networkFound := networksMap[v.NetworkID]
			return networkFound && cntNetwork.Driver == "bridge"
		})

		if exposedHttpPort != 0 {
			s.IdCounter += 1
			s.Endpoints = append(s.Endpoints, Endpoint{
				Id:           s.IdCounter,
				Name:         proposedNames[0],
				Aliases:      proposedNames[1:],
				EndpointType: EndpointContainerType,
				ContainerEndpoint: &ContainerEndpoint{
					ContainerPort:       exposedHttpPort,
					ContainerName:       cnt.Names[0],
					DockerContainer:     cnt.ID,
					DockerContainerName: cnt.Names[0],
				},
				ManualOverride: false,
				ProxyType:      ProxyTypeHttp,
			})
		}

		if cntBridgeNetworkFound {
			cntEndpoint := cnt.NetworkSettings.Networks[cntNetworkKey]
			print("Found its IP address:", cntEndpoint.IPAddress, "will alias the following dnsNames to this IP address ")
			fmt.Printf("%v\n", proposedNames)

			for _, name := range proposedNames {
				dnsNames.Store(name, ContainerTarget{
					Id: cnt.ID, IP: cntEndpoint.IPAddress, Direct: true})
			}

			for _, name := range proposedLoadBalancedNames {
				dnsNames.Store(name, ContainerTarget{Id: cnt.ID, IP: cntEndpoint.IPAddress, Port: exposedHttpPort, Direct: false})
			}
		}
	}
}

// Initialize state from manual overrides.
// Every manual override that is done through the UI or CLI is persisted and is restored on startup.
func initFromManualOverrides() {

}

func handleDockerEvent() {

}

func addEndpoint(endpoint Endpoint) {
	_stateMutationLock.Lock()
	defer _stateMutationLock.Unlock()

	_state.Endpoints = append(_state.Endpoints, endpoint)
	// Run DNS and Proxies for the endpoint
}

package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type ContainerTarget struct {
	Id     string
	Direct bool // Direct access to the container, or through a load balancer
	IP     string
	Port   uint16 // Only initialized if Direct is false (HTTP proxying will be done to this port)
}

const cntTld = "cnt.local."
const lbTld = "lb.cnt.local."

// These are the ports that are commonly used to serve HTTP traffic, we will automatically pick them up if we see them.
// We will consider these ports in the order they are listed here, the first one wins.
var portsToConsiderForProxying = []uint16{80, 8080, 8000, 8010, 443, 3000, 15672}

// Use an unconventional loopback address to avoid conflicts with other potential listeners on :80
var loadBalancerAddress = "127.0.1.99"
var dnsNames = sync.Map{}

func main() {
	dnsServ := &dns.Server{Addr: "127.0.53.35:53", Net: "udp"}
	dnsServ.Handler = dns.HandlerFunc(HandleDnsRequest)

	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	networks, err := docker.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		panic(err)
	}

	networksMap := lo.KeyBy(networks, func(n types.NetworkResource) string { return n.ID })

	containers, err := docker.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, cnt := range containers {
		println("Container:", cnt.ID, "with name", cnt.Names[0], "and labels", cnt.Labels)

		var exposedHttpPort uint16
		for _, port := range cnt.Ports {
			if port.Type == "tcp" && lo.Contains(portsToConsiderForProxying, port.PrivatePort) {
				exposedHttpPort = port.PrivatePort
				break
			}
		}

		proposedNames := make([]string, len(cnt.Names)*5)
		proposedLoadBalancedNames := make([]string, len(cnt.Names)*2)

		for _, name := range cnt.Names {
			normalizedName := normalizeName(name)
			proposedNames = append(proposedNames, normalizedName+".", normalizedName+"."+cntTld)

			if exposedHttpPort != 0 {
				proposedLoadBalancedNames = append(proposedLoadBalancedNames, normalizedName+"."+lbTld)
			}

			// If the name ends with "-1", which happens always with Docker Compose
			if strings.LastIndex(normalizedName, "-1") == len(normalizedName)-2 {
				normalizedName = normalizedName[:len(normalizedName)-2]
				proposedNames = append(proposedNames, normalizedName+".", normalizedName+"."+cntTld)

				if exposedHttpPort != 0 {
					proposedLoadBalancedNames = append(proposedLoadBalancedNames, normalizedName+"."+lbTld)
				}
			}
		}

		composeProject, okProject := cnt.Labels["com.docker.compose.project"]
		serviceName, okService := cnt.Labels["com.docker.compose.service"]
		if okProject && okService {
			println("  Belongs to Compose project", composeProject, "service is", serviceName,
				"suggested DNS name is", serviceName+"."+composeProject+"."+cntTld)
			proposedNames = append(proposedNames, normalizeName(serviceName+"."+composeProject)+"."+cntTld)

			if exposedHttpPort != 0 {
				proposedLoadBalancedNames = append(proposedLoadBalancedNames, normalizeName(serviceName+"."+composeProject)+"."+lbTld)
			}
		}

		cntNetworkKey, cntBridgeNetworkFound := lo.FindKeyBy(cnt.NetworkSettings.Networks, func(k string, v *network.EndpointSettings) bool {
			cntNetwork, networkFound := networksMap[v.NetworkID]
			return networkFound && cntNetwork.Driver == "bridge"
		})

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
	// TODO add docker events listener

	reverseProxy := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			targetContainer, ok := dnsNames.Load(req.Host + ".")
			if ok && !targetContainer.(ContainerTarget).Direct {
				targetIP := targetContainer.(ContainerTarget).IP
				targetPort := targetContainer.(ContainerTarget).Port

				println("Got request to ", req.Host, ", proxying it to", targetIP, "port", targetPort)

				req.URL.Scheme = "http"
				req.URL.Host = targetIP + ":" + strconv.Itoa(int(targetPort))
			} else {
				println("Unroutable HTTP request received: ", req.Host, req.URL.Path, req.URL.RawQuery)
			}
		},
	}
	go http.ListenAndServe(loadBalancerAddress+":80", &reverseProxy)

	dnsServErr := dnsServ.ListenAndServe()
	if dnsServErr != nil {
		panic(dnsServErr)
	}
}

// Regular expression to replace all symbols that are invalid in the domain name to dots
var dnsReplaceSymbolsRegexp = regexp.MustCompile("[^a-zA-Z0-9-.]+")
var dnsReplaceRepeatedSymbols = regexp.MustCompile("([-.])[-.]+")

func normalizeName(name string) string {
	nameWithOnlyValidChars := dnsReplaceSymbolsRegexp.ReplaceAllLiteral([]byte(name), []byte("."))
	nameWithSingleChars := dnsReplaceRepeatedSymbols.ReplaceAll(nameWithOnlyValidChars, []byte("$1"))
	return strings.Trim(string(nameWithSingleChars), ".-")
}

func HandleDnsRequest(w dns.ResponseWriter, r *dns.Msg) {
	println("DNS query received: ", r.Question[0].Name)
	println("ID: ", r.Id)
	println("QType: ", r.Question[0].Qtype)
	println("QClass: ", r.Question[0].Qclass)

	m := new(dns.Msg)
	m.SetReply(r)

	nameTarget, nameExists := dnsNames.Load(r.Question[0].Name)
	if nameExists {
		var ip net.IP
		if nameTarget.(ContainerTarget).Direct {
			ip = net.ParseIP(nameTarget.(ContainerTarget).IP)
		} else {
			ip = net.ParseIP(loadBalancerAddress)
		}

		m.Answer = make([]dns.RR, 1)
		m.Answer[0] = &dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1},
			A:   ip,
		}
	} else {
		// No answer in this case
		// TODO: track dnsNames of containers that have gone away, and return the TXT for them explaining that the container is down.
		textResponse := make([]string, 1)
		textResponse[0] = "Oufff boy we don't resolve this name"

		m.Answer = make([]dns.RR, 1)
		m.Answer[0] = &dns.TXT{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 1},
			Txt: textResponse,
		}
	}

	w.WriteMsg(m)
}

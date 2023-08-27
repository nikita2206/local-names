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
	"regexp"
	"strings"
	"sync"
)

type ContainerTarget struct {
	IP string
	Id string
}

const tld = "cnt.local."

var names = sync.Map{}

func main() {
	dnsServ := &dns.Server{Addr: ":5312", Net: "udp"}
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

	for i := range containers {
		cnt := containers[i]
		println("Container:", cnt.ID, "with name", cnt.Names[0], "and labels", cnt.Labels)

		proposedNames := make([]string, len(cnt.Names)*2)
		for nameIdx := range cnt.Names {
			normalizedName := normalizeName(cnt.Names[nameIdx])
			proposedNames = append(proposedNames, normalizedName+".", normalizedName+"."+tld)

			// If the name ends with "-1", which happens always with Docker Compose
			if strings.LastIndex(normalizedName, "-1") == len(normalizedName)-2 {
				normalizedName = normalizedName[:len(normalizedName)-2]
				proposedNames = append(proposedNames, normalizedName+".", normalizedName+"."+tld)
			}
		}

		composeProject, okProject := cnt.Labels["com.docker.compose.project"]
		serviceName, okService := cnt.Labels["com.docker.compose.service"]
		if okProject && okService {
			println("  Belongs to Compose project", composeProject, "service is", serviceName,
				"suggested DNS name is", serviceName+"."+composeProject+"."+tld)
			proposedNames = append(proposedNames, normalizeName(serviceName+"."+composeProject)+"."+tld)
		}

		cntNetworkKey, cntBridgeNetworkFound := lo.FindKeyBy(cnt.NetworkSettings.Networks, func(k string, v *network.EndpointSettings) bool {
			cntNetwork, networkFound := networksMap[v.NetworkID]
			return networkFound && cntNetwork.Driver == "bridge"
		})

		if cntBridgeNetworkFound {
			cntEndpoint := cnt.NetworkSettings.Networks[cntNetworkKey]
			print("Found its IP address:", cntEndpoint.IPAddress, "will alias the following names to this IP address ")
			fmt.Printf("%v\n", proposedNames)

			for proposedNameIdx := range proposedNames {
				names.Store(proposedNames[proposedNameIdx], ContainerTarget{Id: cnt.ID, IP: cntEndpoint.IPAddress})
			}
		}
	}

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

	nameTarget, nameExists := names.Load(r.Question[0].Name)
	if nameExists {
		m.Answer = make([]dns.RR, 1)
		m.Answer[0] = &dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1},
			A:   net.ParseIP(nameTarget.(ContainerTarget).IP),
		}
	} else {
		// No answer in this case
		// TODO: track names of containers that have gone away, and return the TXT for them explaining that.
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

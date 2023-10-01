package src

import (
	"github.com/docker/docker/client"
	"regexp"
	"strings"
	"sync"
)

var _lastDockerError error
var _dockerClient *client.Client
var _lock = sync.Mutex{}

func Init() {
	_lock.Lock()
	defer _lock.Unlock()

	_dockerClient, _lastDockerError = client.NewClientWithOpts(client.FromEnv)
}

func GetDockerClient() (*client.Client, error) {
	_lock.Lock()
	defer _lock.Unlock()

	if _dockerClient == nil {
		_dockerClient, _lastDockerError = client.NewClientWithOpts(client.FromEnv)
	}

	return _dockerClient, _lastDockerError
}

// Regular expression to replace all symbols that are invalid in the domain name to dots
var _dnsReplaceSymbolsRegexp = regexp.MustCompile("[^a-zA-Z0-9-.]+")
var _dnsReplaceRepeatedSymbols = regexp.MustCompile("([-.])[-.]+")

// NormalizeName replaces all invalid symbols in the domain name with dots, and removes leading and trailing dots.
// Use with Docker container names, that often contain symbols from Docker Compose project names.
func NormalizeName(name string) string {
	nameWithOnlyValidChars := _dnsReplaceSymbolsRegexp.ReplaceAllLiteral([]byte(name), []byte("."))
	nameWithSingleChars := _dnsReplaceRepeatedSymbols.ReplaceAll(nameWithOnlyValidChars, []byte("$1"))
	return strings.Trim(string(nameWithSingleChars), ".-")
}

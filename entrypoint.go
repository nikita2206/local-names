package main

func main() {
	// Initialize the global configuration registry
	//   - init config from file
	//   - init config from environment (docker)
	// Start the DNS servers according to the configuration
	// Start the reverse proxies according to the configuration
	// Start the config file watcher
	//   - every time configuration changes -> merge the changes to the global configuration registry
	// Start the Docker event watcher
	//   - every time a container is started, stopped, or restarted -> update the global configuration registry
	// Start the DNS server and reverse proxy manager, which reacts to changes in the global configuration registry
	//   - every time the config changes, ensure that all DNS servers and reverse proxies are up-to-date
	//   - if some are not up-to-date, kill them and reinitialize them
	//     - maybe more careful approach wrt reverse proxies is needed, due to keep-alive connections

	panic("This file is not supposed to be compiled, it's just a placeholder")
}


I'm a bit struggling with designing architecture in Go, so I'll try to write it down here.

### Architecture

- Everything works based on configuration entities that can be submitted using the API, or read from the Docker daemon.
  - Configuration entity can be derived from the Docker container, e.g. its name an exposed ports.
- A loop that listens to changes in configuration entities, which are sent to its channel.
  - That loop, based on changing configurations, manages the reverse proxy listeners + DNS servers.

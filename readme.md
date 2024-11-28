# Elastic Skeetslurper

skeeSuper basic tooling to slurp sweet skeet juice from Bluesky's Jetstream cluster, index them in Elasticsearch and visualise in Kibana.

Should take about 5 minutes to get up and running, no environment variables to set.

Uses docker-elk forked from https://github.com/deviantony/docker-elk, feel free to swap the git submodule out for your own ELK stack fork.

For more details see --> [What's going on?](##What's-going-on?)

## Prerequisites

- Go 1.22+
- Docker
- ~10Mbps internet connection
- Nothing listening on ports 5601, 9200, 9600


## Setup

### initialise submodules

```
git submodule update --init --recursive
```

### Spin up ELK containers

```
cd elk
docker compose up setup
docker compose up -d
cd ..
```

### Run skeetslurper

```
go run .
```

or

```
go build
chmod +x skeetslurper
./skeetslurper
```


## Open Kibana and take a look

Open the Kibana URL in your browser:

http://localhost:5601/

Login with the following credentials:

**Username:** elastic

**Password:** changeme


Do not expose this outside of your local network, you will instantly get hacked.

## TODO

- [ ] Figure out proper index mapping and how to handle Lexicon/schema changes
- [ ] Some nice prebuilt Bluesky Kibana dashboards

## What's going on?

```mermaid
sequenceDiagram
    participant User
    participant Skeetslurper
    participant Bluesky Servers
    participant Elasticsearch
    participant Kibana

    User->>+Skeetslurper: Start application
    
    rect rgba(191, 223, 255, 0.2)
        Note over Skeetslurper: Initialization Phase
        Skeetslurper->>Bluesky Servers: Ping all Jetstream servers
        loop Server Selection
            Skeetslurper->>Bluesky Servers: Test connection latency
            Bluesky Servers-->>Skeetslurper: Connection response
        end
        Skeetslurper->>Skeetslurper: Select best server
        
        Skeetslurper->>Elasticsearch: Initialize client
        Skeetslurper->>Elasticsearch: Test cluster health
        Elasticsearch-->>Skeetslurper: Cluster status
        
        Skeetslurper->>Elasticsearch: Configure ingest pipelines
        Skeetslurper->>Elasticsearch: Create index template
        Skeetslurper->>Kibana: Create index pattern
    end

    rect rgba(200, 255, 200, 0.2)
        Note over Skeetslurper: Processing Phase
        Skeetslurper->>+Bluesky Servers: Connect to Jetstream WebSocket
        
        loop Message Processing
            Bluesky Servers-->>Skeetslurper: Stream messages
            Skeetslurper->>Skeetslurper: Buffer events
            Skeetslurper->>Elasticsearch: Bulk index messages
            Note over Skeetslurper: Process stats every 30s
        end
    end

    rect rgba(255, 230, 230, 0.2)
        Note over User: Visualization Phase
        User->>+Kibana: Access dashboard (port 5601)
        Kibana->>+User: Request credentials
        User->>+Kibana: Login (elastic/changeme)
        Kibana->>Elasticsearch: Query data
        Elasticsearch-->>Kibana: Return results
        Kibana-->>User: Display visualizations
    end

    rect rgba(255, 255, 200, 0.2)
        Note over Skeetslurper: Shutdown Phase
        User->>Skeetslurper: Interrupt signal (Ctrl+C)
        Skeetslurper->>Elasticsearch: Flush remaining messages
        Skeetslurper->>Skeetslurper: Close connections
        Skeetslurper-->>User: Shutdown complete
    end
```
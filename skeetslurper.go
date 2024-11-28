package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
	"github.com/gorilla/websocket"
)

// Configuration struct
type Config struct {
	JetstreamURL     string
	ElasticsearchURL string
	ElasticsUsername string
	ElasticsPassword string
	IndexPrefix      string
	BatchSize        int
	FlushInterval    time.Duration
	BufferSize       int
	KibanaURL        string
}

// JetstreamEvent represents the basic structure of events from Jetstream
type JetstreamEvent struct {
	Did      string          `json:"did"`
	TimeUs   int64           `json:"time_us"`
	Kind     string          `json:"kind"`
	Commit   json.RawMessage `json:"commit,omitempty"`
	Identity json.RawMessage `json:"identity,omitempty"`
	Account  json.RawMessage `json:"account,omitempty"`
}

func main() {
	// Parse command line flags
	config := parseFlags()
	log.Printf("Starting skeetslurper with config: %+v", config)

	// Initialize Elasticsearch client
	log.Println("Initializing Elasticsearch client...")
	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{config.ElasticsearchURL},
		Username:  config.ElasticsUsername,
		Password:  config.ElasticsPassword,
	})
	if err != nil {
		log.Fatalf("Error creating Elasticsearch client: %s", err)
	}

	// Test the connection
	log.Println("Testing Elasticsearch connection...")
	res, err := es.Cluster.Health(
		es.Cluster.Health.WithContext(context.Background()),
	)
	if err != nil {
		log.Fatalf("Error connecting to Elasticsearch: %s", err)
	}
	defer res.Body.Close()

	// Parse the response
	var clusterHealth struct {
		ClusterName string      `json:"cluster_name"`
		Status      interface{} `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&clusterHealth); err != nil {
		log.Fatalf("Error parsing cluster health response: %s", err)
	}

	log.Printf("Successfully connected to Elasticsearch cluster '%s' (status: %v)",
		clusterHealth.ClusterName,
		clusterHealth.Status)

	// Ensure pipeline definitions exist
	log.Println("Checking and configuring ingest pipelines...")
	if err := ensurePipelines(context.Background(), es, "./pipelines"); err != nil {
		log.Fatalf("Failed to configure required pipelines: %s", err)
	}
	log.Println("Pipeline configuration completed successfully")

	// Ensure index template exists
	if err := ensureIndexTemplate(context.Background(), es, config.IndexPrefix); err != nil {
		log.Fatalf("Failed to create index template: %s", err)
	}
	log.Println("Index template created successfully")

	// Create Kibana index pattern
	log.Println("Creating Kibana index pattern...")
	if err := createKibanaIndexPattern(context.Background(), config); err != nil {
		log.Fatalf("Failed to create Kibana index pattern: %s", err)
	}
	log.Println("Kibana index pattern created successfully")

	// Create bulk indexer
	log.Println("Creating bulk indexer...")
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         config.IndexPrefix + "-" + time.Now().Format("2006.01.02"),
		Client:        es,
		NumWorkers:    2,
		FlushBytes:    5e+6,
		FlushInterval: config.FlushInterval,
		Pipeline:      "bluesky-pipeline",
	})
	if err != nil {
		log.Fatalf("Error creating bulk indexer: %s", err)
	}
	log.Println("Bulk indexer created successfully")

	// Create buffered channel for events
	eventBuffer := make(chan JetstreamEvent, config.BufferSize)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	log.Println("Starting WebSocket consumer...")
	go consumeWebSocket(ctx, config, eventBuffer)

	log.Println("Starting Elasticsearch producer...")
	go produceToElasticsearch(ctx, bi, eventBuffer)

	log.Println("Service started successfully. Press Ctrl+C to exit.")

	// Wait for interrupt signal
	<-sigChan
	log.Println("Received interrupt signal, shutting down...")
	cancel()

	// Close the bulk indexer
	if err := bi.Close(context.Background()); err != nil {
		log.Printf("Error closing bulk indexer: %s", err)
	}
}

func consumeWebSocket(ctx context.Context, config Config, eventBuffer chan<- JetstreamEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("Attempting to connect to WebSocket at %s...", config.JetstreamURL)
			u := url.URL{Scheme: "wss", Host: config.JetstreamURL, Path: "/subscribe"}
			c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Printf("WebSocket dial error: %v, retrying in 5 seconds...", err)
				time.Sleep(5 * time.Second)
				continue
			}
			log.Println("Successfully connected to WebSocket")
			defer c.Close()

			// Add message counter
			messageCount := 0
			lastLog := time.Now()

			// Read messages
			for {
				_, message, err := c.ReadMessage()
				if err != nil {
					log.Printf("WebSocket read error: %v", err)
					break
				}

				messageCount++
				// Log progress every 1000 messages or 30 seconds
				if messageCount%1000 == 0 || time.Since(lastLog) > 30*time.Second {
					log.Printf("Processed %d messages since last report", messageCount)
					messageCount = 0
					lastLog = time.Now()
				}

				var event JetstreamEvent
				if err := json.Unmarshal(message, &event); err != nil {
					log.Printf("JSON unmarshal error: %v", err)
					continue
				}

				select {
				case eventBuffer <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func produceToElasticsearch(ctx context.Context, bi esutil.BulkIndexer, eventBuffer <-chan JetstreamEvent) {
	log.Println("Starting to process events to Elasticsearch")
	stats := struct {
		processed int
		succeeded int
		failed    int
		lastLog   time.Time
	}{
		lastLog: time.Now(),
	}

	// Add stats reporting ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Elasticsearch producer shutting down. Total: %d, Succeeded: %d, Failed: %d",
				stats.processed, stats.succeeded, stats.failed)
			return
		case <-ticker.C:
			// Regular stats reporting
			log.Printf("Elasticsearch stats - Processed: %d, Succeeded: %d, Failed: %d",
				stats.processed, stats.succeeded, stats.failed)
		case event := <-eventBuffer:
			stats.processed++

			// Flatten the commit field if it exists
			if len(event.Commit) > 0 {
				var commitObj map[string]interface{}
				if err := json.Unmarshal(event.Commit, &commitObj); err != nil {
					log.Printf("Error unmarshaling commit: %s", err)
					stats.failed++
					continue
				}

				// Convert complex objects to strings to prevent parsing errors
				if record, ok := commitObj["record"].(map[string]interface{}); ok {
					if subject, ok := record["subject"].(map[string]interface{}); ok {
						jsonStr, err := json.Marshal(subject)
						if err == nil {
							record["subject"] = string(jsonStr)
						}
					}
				}

				eventBytes, err := json.Marshal(commitObj)
				if err == nil {
					event.Commit = eventBytes
				}
			}

			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("Error marshaling event: %s", err)
				stats.failed++
				continue
			}

			err = bi.Add(ctx, esutil.BulkIndexerItem{
				Action: "index",
				Body:   bytes.NewReader(data),
				OnSuccess: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem) {
					stats.succeeded++
				},
				OnFailure: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem, err error) {
					stats.failed++
					if err != nil {
						log.Printf("ERROR: %s", err)
					} else {
						log.Printf("ERROR: %s: %s", res.Error.Type, res.Error.Reason)
					}
				},
			})
			if err != nil {
				log.Printf("Error adding item to bulk indexer: %s", err)
				stats.failed++
			}
		}
	}
}

func parseFlags() Config {
	config := Config{}

	// Add help text for each flag
	flag.StringVar(&config.JetstreamURL, "jetstream", "jetstream2.us-east.bsky.network", 
		"Bluesky Jetstream WebSocket URL to connect to")
	flag.StringVar(&config.ElasticsearchURL, "es", "http://localhost:9200", 
		"Elasticsearch URL including protocol and port")
	flag.StringVar(&config.IndexPrefix, "index", "bluesky-events", 
		"Prefix for Elasticsearch indices. Actual indices will be <prefix>-YYYY.MM.DD")
	flag.IntVar(&config.BatchSize, "batch", 1000, 
		"Number of events to batch together before sending to Elasticsearch")
	flag.DurationVar(&config.FlushInterval, "flush", 30*time.Second, 
		"Maximum time to wait before flushing events to Elasticsearch (e.g. 30s, 1m)")
	flag.IntVar(&config.BufferSize, "buffer", 10000, 
		"Size of the internal event buffer between WebSocket and Elasticsearch")
	flag.StringVar(&config.ElasticsUsername, "es-user", "elastic", 
		"Username for Elasticsearch authentication")
	flag.StringVar(&config.ElasticsPassword, "es-pass", "changeme", 
		"Password for Elasticsearch authentication")
	flag.StringVar(&config.KibanaURL, "kibana", "", 
		"Kibana URL (optional, will be derived from ES URL if not provided)")

	// Add custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "skeetslurper - A tool to stream Bluesky events to Elasticsearch\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -es http://localhost:9200 -es-user elastic -es-pass secret123\n", os.Args[0])
	}

	flag.Parse()

	return config
}

func ensurePipelines(ctx context.Context, es *elasticsearch.Client, pipelinesDir string) error {
	// Read pipeline definitions from directory
	files, err := os.ReadDir(pipelinesDir)
	if err != nil {
		return fmt.Errorf("failed to read pipelines directory %s: %w", pipelinesDir, err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no pipeline definitions found in %s", pipelinesDir)
	}

	log.Printf("Found %d files in pipelines directory", len(files))
	pipelinesProcessed := 0

	for _, file := range files {
		// Skip non-JSON files
		if !strings.HasSuffix(file.Name(), ".json") {
			log.Printf("Skipping non-JSON file: %s", file.Name())
			continue
		}

		// Read pipeline definition
		pipelinePath := filepath.Join(pipelinesDir, file.Name())
		log.Printf("Processing pipeline definition from %s", pipelinePath)

		content, err := os.ReadFile(pipelinePath)
		if err != nil {
			return fmt.Errorf("failed to read pipeline file %s: %w", file.Name(), err)
		}

		// Extract pipeline name from filename (remove .json extension)
		pipelineName := strings.TrimSuffix(file.Name(), ".json")
		log.Printf("Checking pipeline: %s", pipelineName)

		// Parse new pipeline content
		var newPipeline map[string]interface{}
		if err := json.Unmarshal(content, &newPipeline); err != nil {
			return fmt.Errorf("failed to parse pipeline definition %s: %w", pipelineName, err)
		}

		// Get new version
		newVersion, hasVersion := newPipeline["version"].(float64)
		if !hasVersion {
			return fmt.Errorf("pipeline %s missing version field", pipelineName)
		}

		// Check if pipeline exists
		res, err := es.Ingest.GetPipeline(
			es.Ingest.GetPipeline.WithContext(ctx),
			es.Ingest.GetPipeline.WithHuman(),
			es.Ingest.GetPipeline.WithPipelineID(pipelineName),
		)
		if err != nil {
			return fmt.Errorf("failed to check pipeline %s: %w", pipelineName, err)
		}

		needsUpdate := true
		if res.StatusCode != 404 {
			var existingPipeline map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&existingPipeline); err != nil {
				res.Body.Close()
				return fmt.Errorf("failed to decode existing pipeline %s: %w", pipelineName, err)
			}
			res.Body.Close()

			if pipelineData, exists := existingPipeline[pipelineName].(map[string]interface{}); exists {
				if existingVersion, hasVersion := pipelineData["version"].(float64); hasVersion {
					if existingVersion >= newVersion {
						log.Printf("Pipeline %s already exists with same or newer version (existing: %.1f, new: %.1f)",
							pipelineName, existingVersion, newVersion)
						needsUpdate = false
					} else {
						log.Printf("Updating pipeline %s from version %.1f to %.1f",
							pipelineName, existingVersion, newVersion)
					}
				}
			}
		} else {
			log.Printf("Creating new pipeline %s with version %.1f", pipelineName, newVersion)
		}

		if needsUpdate {
			res, err = es.Ingest.PutPipeline(
				pipelineName,
				strings.NewReader(string(content)),
				es.Ingest.PutPipeline.WithContext(ctx),
			)
			if err != nil {
				return fmt.Errorf("failed to create/update pipeline %s: %w", pipelineName, err)
			}
			if res.IsError() {
				return fmt.Errorf("error creating/updating pipeline %s: %s", pipelineName, res.String())
			}
			log.Printf("Pipeline %s version %.1f created/updated successfully", pipelineName, newVersion)
		}

		pipelinesProcessed++
	}

	if pipelinesProcessed == 0 {
		return fmt.Errorf("no valid pipeline definitions were processed")
	}

	log.Printf("Successfully processed %d pipeline definitions", pipelinesProcessed)
	return nil
}

func createKibanaIndexPattern(ctx context.Context, config Config) error {
	// Parse Elasticsearch URL to get base URL for Kibana
	esURL, err := url.Parse(config.ElasticsearchURL)
	if err != nil {
		return fmt.Errorf("failed to parse ES URL: %w", err)
	}

	// Construct Kibana URL
	kibanaURL := url.URL{
		Scheme: esURL.Scheme,
		Host:   strings.Replace(esURL.Host, "9200", "5601", 1),
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Define pattern for Kibana 8.x
	newPattern := map[string]interface{}{
		"attributes": map[string]interface{}{
			"title":         config.IndexPrefix + "-*",
			"timeFieldName": "@timestamp",
			"fields":        "[]",
			"fieldFormats":  "{}",
			"sourceFilters": "[]",
			"fieldAttrs":    "{}",
		},
	}

	// Marshal pattern to JSON
	patternJSON, err := json.Marshal(newPattern)
	if err != nil {
		return fmt.Errorf("failed to marshal pattern: %w", err)
	}

	// Add overwrite parameter to the URL
	createReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		kibanaURL.String()+"/api/saved_objects/index-pattern/"+url.PathEscape(config.IndexPrefix)+"?overwrite=true",
		bytes.NewReader(patternJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("kbn-xsrf", "true")
	if config.ElasticsUsername != "" {
		createReq.SetBasicAuth(config.ElasticsUsername, config.ElasticsPassword)
	}

	// Send the request
	createResp, err := client.Do(createReq)
	if err != nil {
		return fmt.Errorf("failed to send create request: %w", err)
	}
	defer createResp.Body.Close()

	// Check response
	if createResp.StatusCode >= 400 {
		body, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("failed to create/update index pattern: status=%d body=%s", createResp.StatusCode, string(body))
	}

	log.Println("Successfully created/updated Kibana index pattern")
	return nil
}

func ensureIndexTemplate(ctx context.Context, es *elasticsearch.Client, indexPrefix string) error {
	// Update the path to use ./index_maps directory
	content, err := os.ReadFile("./index_maps/index_template.json")
	if err != nil {
		return fmt.Errorf("failed to read index template file: %w", err)
	}

	// Rest of the function remains the same
	var template map[string]interface{}
	if err := json.Unmarshal(content, &template); err != nil {
		return fmt.Errorf("failed to parse template JSON: %w", err)
	}

	if patterns, ok := template["index_patterns"].([]interface{}); ok && len(patterns) > 0 {
		template["index_patterns"] = []string{indexPrefix + "-*"}
	}

	templateJSON, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	res, err := es.Indices.PutIndexTemplate(
		indexPrefix+"-template",
		bytes.NewReader(templateJSON),
		es.Indices.PutIndexTemplate.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("error creating template: %s", string(body))
	}

	return nil
}

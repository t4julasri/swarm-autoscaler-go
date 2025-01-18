package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

const (
	cpuPercentageUpperLimit = 85.0
	cpuPercentageLowerLimit = 25.0
	prometheusAPI           = "api/v1/query?query="
	prometheusQuery         = `sum(rate(container_cpu_usage_seconds_total{container_label_com_docker_swarm_task_name=~'.+'}[1m]))BY(container_label_com_docker_swarm_service_name,instance)*100`
)

type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				ContainerLabelComDockerSwarmServiceName string `json:"container_label_com_docker_swarm_service_name"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type ServiceAutoscaler struct {
	dockerClient  *client.Client
	prometheusURL string
	checkInterval time.Duration
}

func NewServiceAutoscaler(prometheusURL string, checkInterval time.Duration) (*ServiceAutoscaler, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %v", err)
	}

	return &ServiceAutoscaler{
		dockerClient:  dockerClient,
		prometheusURL: prometheusURL,
		checkInterval: checkInterval,
	}, nil

}

func createPrometheusClient() *http.Client {
	// Create a custom dialer with Docker DNS settings
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return net.Dial("udp", "127.0.0.11:53")
			},
		},
	}

	// Create transport with the custom dialer
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Create client with the custom transport
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

func (sa *ServiceAutoscaler) getPrometheusMetrics() (*PrometheusResponse, error) {
	queryURL := fmt.Sprintf("%s/%s%s", sa.prometheusURL, prometheusAPI, url.QueryEscape(prometheusQuery))

	// Use the custom client
	client := createPrometheusClient()
	resp, err := client.Get(queryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get prometheus metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var prometheusResp PrometheusResponse
	if err := json.Unmarshal(body, &prometheusResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prometheus response: %v", err)
	}

	return &prometheusResp, nil
}

func (sa *ServiceAutoscaler) getServiceInfo(serviceName string) (*swarm.Service, error) {
	service, _, err := sa.dockerClient.ServiceInspectWithRaw(context.Background(), serviceName, types.ServiceInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect service %s: %v", serviceName, err)
	}
	return &service, nil
}

func (sa *ServiceAutoscaler) scaleService(serviceName string, replicas uint64) error {
	service, err := sa.getServiceInfo(serviceName)
	if err != nil {
		return err
	}

	serviceSpec := service.Spec
	serviceSpec.Mode.Replicated.Replicas = &replicas

	sa.dockerClient.ServiceUpdate(
		context.Background(),
		service.ID,
		service.Version,
		serviceSpec,
		types.ServiceUpdateOptions{},
	)

	return nil
}

func (sa *ServiceAutoscaler) handleService(service *swarm.Service, cpuUsage float64) error {
	if service.Spec.Labels["swarm.autoscaler"] != "true" {
		return nil
	}

	minReplicas, _ := strconv.ParseUint(service.Spec.Labels["swarm.autoscaler.minimum"], 10, 64)
	maxReplicas, _ := strconv.ParseUint(service.Spec.Labels["swarm.autoscaler.maximum"], 10, 64)
	currentReplicas := *service.Spec.Mode.Replicated.Replicas

	if cpuUsage > cpuPercentageUpperLimit && currentReplicas < maxReplicas {
		newReplicas := currentReplicas + 2 // Scale up by 2
		if newReplicas > maxReplicas {
			newReplicas = maxReplicas
		}
		log.Printf("Scaling up service %s from %d to %d replicas (CPU: %.2f%%)\n",
			service.Spec.Name, currentReplicas, newReplicas, cpuUsage)
		return sa.scaleService(service.Spec.Name, newReplicas)
	}

	if cpuUsage < cpuPercentageLowerLimit && currentReplicas > minReplicas {
		newReplicas := currentReplicas - 1 // Scale down by 1
		if newReplicas < minReplicas {
			newReplicas = minReplicas
		}
		log.Printf("Scaling down service %s from %d to %d replicas (CPU: %.2f%%)\n",
			service.Spec.Name, currentReplicas, newReplicas, cpuUsage)
		return sa.scaleService(service.Spec.Name, newReplicas)
	}

	return nil
}

func (sa *ServiceAutoscaler) run() {
	for {
		metrics, err := sa.getPrometheusMetrics()
		if err != nil {
			log.Printf("Error getting metrics: %v", err)
			time.Sleep(sa.checkInterval)
			continue
		}

		services, err := sa.dockerClient.ServiceList(context.Background(), types.ServiceListOptions{})
		if err != nil {
			log.Printf("Error listing services: %v", err)
			time.Sleep(sa.checkInterval)
			continue
		}

		for _, result := range metrics.Data.Result {
			serviceName := result.Metric.ContainerLabelComDockerSwarmServiceName
			cpuUsage, _ := strconv.ParseFloat(result.Value[1].(string), 64)

			for _, service := range services {
				if service.Spec.Name == serviceName {
					if err := sa.handleService(&service, cpuUsage); err != nil {
						log.Printf("Error handling service %s: %v", serviceName, err)
					}
					break
				}
			}
		}

		log.Printf("Waiting %v before next check\n", sa.checkInterval)
		time.Sleep(sa.checkInterval)
	}
}

func main() {
	prometheusURL := os.Getenv("PROMETHEUS_URL")

	fmt.Println(prometheusURL)
	if prometheusURL == "" {
		log.Fatal("PROMETHEUS_URL environment variable is required")
	}

	checkInterval := 60 * time.Second
	if intervalStr := os.Getenv("CHECK_INTERVAL"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil {
			checkInterval = time.Duration(interval) * time.Second
		}
	}

	autoscaler, err := NewServiceAutoscaler(prometheusURL, checkInterval)
	if err != nil {
		log.Fatalf("Failed to create autoscaler: %v", err)
	}

	log.Printf("Starting autoscaler with check interval of %v\n", checkInterval)
	autoscaler.run()
}

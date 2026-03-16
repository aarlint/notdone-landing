package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// --- Metrics API types (metrics.k8s.io/v1beta1) ---

type metricsNodeList struct {
	Items []metricsNode `json:"items"`
}

type metricsNode struct {
	Metadata metav1.ObjectMeta  `json:"metadata"`
	Usage    metricsNodeUsage   `json:"usage"`
}

type metricsNodeUsage struct {
	CPU    resource.Quantity `json:"cpu"`
	Memory resource.Quantity `json:"memory"`
}

type metricsPodList struct {
	Items []metricsPod `json:"items"`
}

type metricsPod struct {
	Metadata   metav1.ObjectMeta     `json:"metadata"`
	Containers []metricsPodContainer `json:"containers"`
}

type metricsPodContainer struct {
	Name  string            `json:"name"`
	Usage metricsNodeUsage  `json:"usage"`
}

// --- Response types ---

type NodeMetrics struct {
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	IP         string  `json:"ip"`
	CPUCores   int     `json:"cpuCores"`
	MemTotalMi int64   `json:"memTotalMi"`
	CPUUsedM   int64   `json:"cpuUsedM"`
	MemUsedMi  int64   `json:"memUsedMi"`
	CPUPercent float64 `json:"cpuPercent"`
	MemPercent float64 `json:"memPercent"`
	PodCount   int     `json:"podCount"`
}

type PodMetrics struct {
	Name      string `json:"name"`
	Node      string `json:"node"`
	CPUUsedM  int64  `json:"cpuUsedM"`
	MemUsedMi int64  `json:"memUsedMi"`
	Restarts  int32  `json:"restarts"`
}

type MetricsResponse struct {
	UpdatedAt         int64                  `json:"updatedAt"`
	Nodes             []NodeMetrics          `json:"nodes"`
	Pods              map[string]*PodMetrics `json:"pods"`
	ClusterCPUPercent float64                `json:"clusterCpuPercent"`
	ClusterMemPercent float64                `json:"clusterMemPercent"`
	TotalCPUCores     int                    `json:"totalCpuCores"`
	TotalMemMi        int64                  `json:"totalMemMi"`
}

// --- Collector ---

type MetricsCollector struct {
	mu         sync.RWMutex
	data       *MetricsResponse
	clientset  kubernetes.Interface
	restClient rest.Interface
	registry   *AppRegistry
}

func NewMetricsCollector(cs kubernetes.Interface, config *rest.Config, registry *AppRegistry) (*MetricsCollector, error) {
	cfg := rest.CopyConfig(config)
	cfg.APIPath = "/apis"
	cfg.GroupVersion = &metricsPodGroupVersion
	cfg.NegotiatedSerializer = codecs

	rc, err := rest.RESTClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("metrics rest client: %w", err)
	}

	return &MetricsCollector{
		clientset:  cs,
		restClient: rc,
		registry:   registry,
		data: &MetricsResponse{
			UpdatedAt: time.Now().Unix(),
			Nodes:     []NodeMetrics{},
			Pods:      make(map[string]*PodMetrics),
		},
	}, nil
}

func (mc *MetricsCollector) RunPoller(ctx context.Context, interval time.Duration) {
	mc.collect(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mc.collect(ctx)
		}
	}
}

func (mc *MetricsCollector) collect(parentCtx context.Context) {
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
	defer cancel()

	// Fetch all data in parallel
	var (
		nodeMetricsList metricsNodeList
		podMetricsList  metricsPodList
		nodeList        *corev1.NodeList
		podList         *corev1.PodList
		wg              sync.WaitGroup
		errNodeMetrics  error
		errPodMetrics   error
		errNodes        error
		errPods         error
	)

	wg.Add(4)
	go func() {
		defer wg.Done()
		raw, err := mc.restClient.Get().
			AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").
			Do(ctx).Raw()
		if err != nil {
			errNodeMetrics = err
			return
		}
		errNodeMetrics = json.Unmarshal(raw, &nodeMetricsList)
	}()
	go func() {
		defer wg.Done()
		raw, err := mc.restClient.Get().
			AbsPath("/apis/metrics.k8s.io/v1beta1/pods").
			Do(ctx).Raw()
		if err != nil {
			errPodMetrics = err
			return
		}
		errPodMetrics = json.Unmarshal(raw, &podMetricsList)
	}()
	go func() {
		defer wg.Done()
		nodeList, errNodes = mc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	}()
	go func() {
		defer wg.Done()
		podList, errPods = mc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	}()
	wg.Wait()

	if errNodeMetrics != nil {
		log.Printf("metrics: node metrics fetch failed: %v", errNodeMetrics)
		return
	}
	if errPodMetrics != nil {
		log.Printf("metrics: pod metrics fetch failed: %v", errPodMetrics)
		return
	}
	if errNodes != nil {
		log.Printf("metrics: node list failed: %v", errNodes)
		return
	}
	if errPods != nil {
		log.Printf("metrics: pod list failed: %v", errPods)
		return
	}

	// Build node capacity map
	type nodeInfo struct {
		cpuCores   int
		memTotalMi int64
		role       string
		ip         string
	}
	nodeInfoMap := make(map[string]nodeInfo, len(nodeList.Items))
	for _, n := range nodeList.Items {
		cpuCores := int(n.Status.Capacity.Cpu().Value())
		memBytes := n.Status.Capacity.Memory().Value()
		memMi := memBytes / (1024 * 1024)

		role := "agent"
		if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
			role = "server"
		}
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "server"
		}

		var ip string
		for _, addr := range n.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				ip = addr.Address
				break
			}
		}

		nodeInfoMap[n.Name] = nodeInfo{
			cpuCores:   cpuCores,
			memTotalMi: memMi,
			role:       role,
			ip:         ip,
		}
	}

	// Count pods per node (only Running pods from our registry apps)
	registryApps := mc.registry.Apps()
	registryKeys := make(map[string]bool, len(registryApps))
	for _, a := range registryApps {
		registryKeys[a.Key] = true
	}

	// Count all running pods per node
	nodePodCount := make(map[string]int)
	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodRunning {
			nodePodCount[p.Spec.NodeName]++
		}
	}

	// Build pod details map keyed by deployment name
	podDetails := make(map[string]*corev1.Pod)
	for i, p := range podList.Items {
		appLabel := p.Labels["app"]
		if appLabel != "" && registryKeys[appLabel] {
			// Keep the first running pod per app
			if _, exists := podDetails[appLabel]; !exists {
				podDetails[appLabel] = &podList.Items[i]
			}
		}
	}

	// Build pod metrics map keyed by pod name
	podMetricsMap := make(map[string]*metricsPod, len(podMetricsList.Items))
	for i, pm := range podMetricsList.Items {
		podMetricsMap[pm.Metadata.Name] = &podMetricsList.Items[i]
	}

	// Build node metrics
	var totalCPUCores int
	var totalMemMi int64
	var totalCPUUsedM int64
	var totalMemUsedMi int64

	nodes := make([]NodeMetrics, 0, len(nodeMetricsList.Items))
	for _, nm := range nodeMetricsList.Items {
		info, ok := nodeInfoMap[nm.Metadata.Name]
		if !ok {
			continue
		}
		cpuUsedM := nm.Usage.CPU.MilliValue()
		memUsedBytes := nm.Usage.Memory.Value()
		memUsedMi := memUsedBytes / (1024 * 1024)

		cpuTotalM := int64(info.cpuCores) * 1000
		var cpuPct, memPct float64
		if cpuTotalM > 0 {
			cpuPct = math.Round(float64(cpuUsedM)/float64(cpuTotalM)*1000) / 10
		}
		if info.memTotalMi > 0 {
			memPct = math.Round(float64(memUsedMi)/float64(info.memTotalMi)*1000) / 10
		}

		totalCPUCores += info.cpuCores
		totalMemMi += info.memTotalMi
		totalCPUUsedM += cpuUsedM
		totalMemUsedMi += memUsedMi

		nodes = append(nodes, NodeMetrics{
			Name:       nm.Metadata.Name,
			Role:       info.role,
			IP:         info.ip,
			CPUCores:   info.cpuCores,
			MemTotalMi: info.memTotalMi,
			CPUUsedM:   cpuUsedM,
			MemUsedMi:  memUsedMi,
			CPUPercent: cpuPct,
			MemPercent: memPct,
			PodCount:   nodePodCount[nm.Metadata.Name],
		})
	}

	// Cluster-level percentages
	var clusterCPUPct, clusterMemPct float64
	totalCPUM := int64(totalCPUCores) * 1000
	if totalCPUM > 0 {
		clusterCPUPct = math.Round(float64(totalCPUUsedM)/float64(totalCPUM)*1000) / 10
	}
	if totalMemMi > 0 {
		clusterMemPct = math.Round(float64(totalMemUsedMi)/float64(totalMemMi)*1000) / 10
	}

	// Build per-app pod metrics
	pods := make(map[string]*PodMetrics, len(registryApps))
	for _, app := range registryApps {
		pod, ok := podDetails[app.Key]
		if !ok {
			continue
		}

		pm := &PodMetrics{
			Name: pod.Name,
			Node: pod.Spec.NodeName,
		}

		// Sum restarts across containers
		for _, cs := range pod.Status.ContainerStatuses {
			pm.Restarts += cs.RestartCount
		}

		// Sum CPU/mem from metrics
		if mPod, ok := podMetricsMap[pod.Name]; ok {
			for _, c := range mPod.Containers {
				pm.CPUUsedM += c.Usage.CPU.MilliValue()
				memBytes := c.Usage.Memory.Value()
				pm.MemUsedMi += memBytes / (1024 * 1024)
			}
		}

		pods[app.Key] = pm
	}

	resp := &MetricsResponse{
		UpdatedAt:         time.Now().Unix(),
		Nodes:             nodes,
		Pods:              pods,
		ClusterCPUPercent: clusterCPUPct,
		ClusterMemPercent: clusterMemPct,
		TotalCPUCores:     totalCPUCores,
		TotalMemMi:        totalMemMi,
	}

	mc.mu.Lock()
	mc.data = resp
	mc.mu.Unlock()

	log.Printf("metrics: collected %d nodes, %d pods", len(nodes), len(pods))
}

func (mc *MetricsCollector) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	mc.mu.RLock()
	data := mc.data
	mc.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AppStatus holds the health check result for a single app.
type AppStatus struct {
	Status  string `json:"status"`
	Ready   int32  `json:"ready"`
	Desired int32  `json:"desired"`
	HTTPOK  bool   `json:"httpOk"`
}

// StatusResponse is the JSON payload for /api/status.
type StatusResponse struct {
	UpdatedAt int64                 `json:"updatedAt"`
	Apps      map[string]*AppStatus `json:"apps"`
}

// StatusCache holds cached health data with concurrent-safe access.
type StatusCache struct {
	mu         sync.RWMutex
	data       *StatusResponse
	clientset  kubernetes.Interface
	httpClient *http.Client
	registry   *AppRegistry
}

// NewStatusCache creates a new cache with the given k8s client and app registry.
func NewStatusCache(cs kubernetes.Interface, registry *AppRegistry) *StatusCache {
	return &StatusCache{
		clientset:  cs,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		registry:   registry,
		data: &StatusResponse{
			UpdatedAt: time.Now().Unix(),
			Apps:      make(map[string]*AppStatus),
		},
	}
}

// JSON returns the cached status as JSON bytes.
func (sc *StatusCache) JSON() ([]byte, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return json.Marshal(sc.data)
}

// RunPoller starts the background health check loop.
func (sc *StatusCache) RunPoller(ctx context.Context, interval time.Duration) {
	sc.checkAll(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sc.checkAll(ctx)
		}
	}
}

func (sc *StatusCache) checkAll(parentCtx context.Context) {
	apps := sc.registry.Apps()

	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()

	results := make(map[string]*AppStatus, len(apps))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, app := range apps {
		wg.Add(1)
		go func(a AppDef) {
			defer wg.Done()
			s := sc.checkOne(ctx, a)
			mu.Lock()
			results[a.Key] = s
			mu.Unlock()
		}(app)
	}
	wg.Wait()

	sc.mu.Lock()
	sc.data = &StatusResponse{
		UpdatedAt: time.Now().Unix(),
		Apps:      results,
	}
	sc.mu.Unlock()
}

func (sc *StatusCache) checkOne(ctx context.Context, app AppDef) *AppStatus {
	s := &AppStatus{Status: "unknown"}

	if sc.clientset != nil {
		deploy, err := sc.clientset.AppsV1().Deployments(app.Namespace).Get(ctx, app.Deploy, metav1.GetOptions{})
		if err != nil {
			log.Printf("k8s check failed for %s: %v", app.Key, err)
			return s
		}
		s.Desired = *deploy.Spec.Replicas
		s.Ready = deploy.Status.ReadyReplicas

		if s.Desired == 0 {
			s.Status = "unknown"
			return s
		}
		if s.Ready == 0 {
			s.Status = "unhealthy"
			return s
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, app.SvcURL, nil)
	if err != nil {
		s.Status = deriveStatus(s.Ready, s.Desired, false)
		return s
	}
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		s.HTTPOK = false
		s.Status = deriveStatus(s.Ready, s.Desired, false)
		return s
	}
	resp.Body.Close()
	s.HTTPOK = resp.StatusCode >= 200 && resp.StatusCode < 400
	s.Status = deriveStatus(s.Ready, s.Desired, s.HTTPOK)
	return s
}

func deriveStatus(ready, desired int32, httpOK bool) string {
	if desired == 0 {
		return "unknown"
	}
	if ready == 0 {
		return "unhealthy"
	}
	if ready == desired && httpOK {
		return "healthy"
	}
	return "degraded"
}

// ServeStatus is the HTTP handler for /api/status.
func (sc *StatusCache) ServeStatus(w http.ResponseWriter, r *http.Request) {
	data, err := sc.JSON()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

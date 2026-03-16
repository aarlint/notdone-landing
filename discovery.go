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

// AppDef defines an app deployed to the cluster.
type AppDef struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Desc     string `json:"desc"`
	URL      string `json:"url"`
	Repo     string `json:"repo"`
	Category string `json:"category"`

	Namespace string `json:"-"`
	Deploy    string `json:"-"`
	SvcURL    string `json:"-"`
}

// categoryOrder returns a deduplicated list of categories from the given apps,
// preserving the order of first appearance.
func categoryOrder(apps []AppDef) []string {
	seen := map[string]bool{}
	var cats []string
	for _, a := range apps {
		if a.Category != "" && !seen[a.Category] {
			seen[a.Category] = true
			cats = append(cats, a.Category)
		}
	}
	return cats
}

const (
	labelSelector = "notdone.dev/app=true"
	annoPrefix    = "notdone.dev/"
)

// AppsResponse is the JSON payload for /api/apps.
type AppsResponse struct {
	Apps       []AppDef `json:"apps"`
	Categories []string `json:"categories"`
}

// AppRegistry discovers apps from k8s deployment annotations.
type AppRegistry struct {
	mu        sync.RWMutex
	apps      []AppDef
	clientset kubernetes.Interface
	ready     chan struct{}
}

// NewAppRegistry creates a registry with the given k8s client.
func NewAppRegistry(cs kubernetes.Interface) *AppRegistry {
	return &AppRegistry{
		clientset: cs,
		ready:     make(chan struct{}),
	}
}

// Ready returns a channel that closes after the first successful refresh.
func (r *AppRegistry) Ready() <-chan struct{} {
	return r.ready
}

// Apps returns the current snapshot of discovered apps.
func (r *AppRegistry) Apps() []AppDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AppDef, len(r.apps))
	copy(out, r.apps)
	return out
}

// RunRefresh performs an initial refresh then refreshes every interval.
func (r *AppRegistry) RunRefresh(ctx context.Context, interval time.Duration) {
	r.refresh(ctx)
	close(r.ready)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refresh(ctx)
		}
	}
}

func (r *AppRegistry) refresh(ctx context.Context) {
	if r.clientset == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	deployments, err := r.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		log.Printf("discovery: failed to list deployments: %v", err)
		return
	}

	apps := make([]AppDef, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		a := d.Annotations
		name := a[annoPrefix+"name"]
		url := a[annoPrefix+"url"]
		if name == "" || url == "" {
			log.Printf("discovery: skipping %s/%s — missing name or url annotation", d.Namespace, d.Name)
			continue
		}

		apps = append(apps, AppDef{
			Key:       d.Name,
			Name:      name,
			Icon:      a[annoPrefix+"icon"],
			Desc:      a[annoPrefix+"desc"],
			URL:       url,
			Repo:      a[annoPrefix+"repo"],
			Category:  a[annoPrefix+"category"],
			Namespace: d.Namespace,
			Deploy:    d.Name,
			SvcURL:    fmt.Sprintf("http://%s.%s.svc.cluster.local", d.Name, d.Namespace),
		})
	}

	log.Printf("discovery: found %d apps", len(apps))

	r.mu.Lock()
	r.apps = apps
	r.mu.Unlock()
}

// ServeApps is the HTTP handler for /api/apps.
func (r *AppRegistry) ServeApps(w http.ResponseWriter, _ *http.Request) {
	apps := r.Apps()
	resp := AppsResponse{
		Apps:       apps,
		Categories: categoryOrder(apps),
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	json.NewEncoder(w).Encode(resp)
}

package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed index.html
var staticFS embed.FS

func main() {
	var cs kubernetes.Interface
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Not in cluster (%v), trying kubeconfig", err)
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Printf("No kubeconfig available: %v — running without k8s checks", err)
		}
	}
	if config != nil {
		cs, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Printf("Failed to create k8s client: %v — running without k8s checks", err)
		}
	}

	registry := NewAppRegistry(cs)
	cache := NewStatusCache(cs, registry)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go registry.RunRefresh(ctx, 60*time.Second)
	<-registry.Ready()
	go cache.RunPoller(ctx, 30*time.Second)

	var metrics *MetricsCollector
	if config != nil && cs != nil {
		var mErr error
		metrics, mErr = NewMetricsCollector(cs, config, registry)
		if mErr != nil {
			log.Printf("Failed to create metrics collector: %v — running without metrics", mErr)
		} else {
			go metrics.RunPoller(ctx, 30*time.Second)
		}
	}

	mux := http.NewServeMux()

	indexHTML, _ := staticFS.ReadFile("index.html")

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "notdone.dev" {
			http.Redirect(w, r, "https://apps.notdone.dev"+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		w.Write(indexHTML)
	})

	mux.HandleFunc("/api/apps", registry.ServeApps)
	mux.HandleFunc("/api/status", cache.ServeStatus)
	if metrics != nil {
		mux.HandleFunc("/api/metrics", metrics.ServeMetrics)
	}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Printf("Listening on :%s", port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
}

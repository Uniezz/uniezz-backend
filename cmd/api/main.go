package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/health"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/server"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

const (
	_shutdownPeriod      = 15 * time.Second
	_readinessDrainDelay = 5 * time.Second
)

var isShuttingDown atomic.Bool

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
		MaxAge:         300,
	})
}

func realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Real-IP"); net.ParseIP(ip) != nil {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

func newRouter(cfg *config.Config) (*chi.Mux, huma.API) {
	r := chi.NewRouter()
	r.Use(realIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.CORSAllowedOrigins))

	return r, humachi.New(r, httpapi.NewConfig(cfg))
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer initCancel()

	dbPool, err := database.NewPool(initCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
	}
	defer dbPool.Close()

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r, api := newRouter(cfg)

	health.Register(r, &isShuttingDown, dbPool)

	modules, err := server.New(dbPool, cfg)
	if err != nil {
		log.Fatalf("Error setting up modules: %v", err)
	}
	modules.Register(api)

	ongoingCtx, stopOngoingGracefully := context.WithCancel(context.Background())
	server := &http.Server{
		Addr:         net.JoinHostPort("", cfg.Port),
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ongoingCtx
		},
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("Starting server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		log.Fatalf("Server error: %v", err)
		return
	case <-rootCtx.Done():
		log.Println("Shutdown signal received, initiating graceful shutdown...")
	}

	stop()
	isShuttingDown.Store(true)
	log.Println("Shutdown signal received, waiting for ongoing requests to finish...")

	log.Printf("Waiting for %s before starting shutdown...", _readinessDrainDelay)
	time.Sleep(_readinessDrainDelay)

	log.Println("Now waiting for ongoing requests to finish...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), _shutdownPeriod)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown timed out or failed: %v", err)
		_ = server.Close()
	}

	stopOngoingGracefully()
	log.Println("Server gracefully stopped")
}

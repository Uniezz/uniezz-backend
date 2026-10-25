package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/health"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/server"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

const (
	_shutdownPeriod       = 15 * time.Second
	_readinessDrainDelay  = 5 * time.Second
	_telemetryFlushPeriod = 5 * time.Second
)

var isShuttingDown atomic.Bool

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
		ExposedHeaders: []string{telemetry.RequestIDHeader},
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

func readyURL(port string) string {
	return "http://" + net.JoinHostPort("127.0.0.1", cmp.Or(port, "8080")) + "/healthz/ready"
}

func checkHealth(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return nil
}

func newRouter(cfg *config.Config) (*chi.Mux, huma.API) {
	r := chi.NewRouter()
	r.Use(realIP)
	r.Use(telemetry.Middleware)
	r.Use(corsMiddleware(cfg.CORSAllowedOrigins))

	return r, humachi.New(r, httpapi.NewConfig(cfg))
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check that the running server is ready and exit")
	flag.Parse()

	if *healthcheck {
		if err := checkHealth(readyURL(os.Getenv("PORT"))); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
			os.Exit(1)
		}
		return
	}

	slog.SetDefault(telemetry.NewLogger(os.Stdout, telemetry.Resource{
		Service: config.DefaultServiceName,
		Env:     cmp.Or(os.Getenv("ENV"), "development"),
		Version: httpapi.Version,
	}))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	tel, err := telemetry.Setup(context.Background(), telemetry.Options{
		Resource:          telemetry.Resource{Service: cfg.ServiceName, Env: cfg.Env, Version: httpapi.Version},
		SentryDSN:         cfg.SentryDSN,
		SentryEnvironment: cfg.SentryEnvironment,
		SentryRelease:     cfg.SentryRelease,
		Stdout:            os.Stdout,
	})
	if err != nil {
		slog.Error("set up telemetry", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(tel.Logger)

	runErr := run(cfg)
	if runErr != nil {
		slog.Error("server stopped", "error", runErr)
	}

	flushCtx, cancel := context.WithTimeout(context.Background(), _telemetryFlushPeriod)
	defer cancel()
	if err := tel.Shutdown(flushCtx); err != nil {
		slog.Warn("flush telemetry", "error", err)
	}

	if runErr != nil {
		os.Exit(1)
	}
}

func run(cfg *config.Config) error {
	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer initCancel()

	dbPool, err := database.NewPool(initCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to the database: %w", err)
	}
	defer dbPool.Close()

	mediaStorage, err := media.New(initCtx, cfg)
	if err != nil {
		return fmt.Errorf("connect to media storage: %w", err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r, api := newRouter(cfg)

	health.Register(r, &isShuttingDown, dbPool, mediaStorage)

	modules, err := server.New(dbPool, cfg)
	if err != nil {
		return fmt.Errorf("set up modules: %w", err)
	}
	modules.Register(api)

	ongoingCtx, stopOngoingGracefully := context.WithCancel(context.Background())
	defer stopOngoingGracefully()
	server := &http.Server{
		Addr:         net.JoinHostPort("", cfg.Port),
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ongoingCtx
		},
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}

	serverErrors := make(chan error, 1)

	go func() {
		slog.Info("starting server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("serve: %w", err)
	case <-rootCtx.Done():
	}

	stop()
	isShuttingDown.Store(true)
	slog.Info("shutdown signal received, draining", "delay", _readinessDrainDelay.String())
	time.Sleep(_readinessDrainDelay)

	slog.Info("waiting for ongoing requests to finish")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), _shutdownPeriod)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown timed out or failed", "error", err)
		_ = server.Close()
	}

	stopOngoingGracefully()
	slog.Info("server gracefully stopped")
	return nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BjoernFuertges/Socioeconomic-Data/backend/docs"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/config"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/handlers"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/routes"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/version"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// @title Sozioökonomische Daten API
// @version dev
// @description REST-API für LAU-Geodaten und Arbeitslosenzahlen aus dbt-transformierten Daten
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email Socioeconomic-Data@b.sgtrf.de

// @BasePath /api/v1
// @schemes http
func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// config.Load guarantees these are set (they are all listed in .env.example),
	// so there are deliberately no fallback credentials here.
	apiHost := os.Getenv("API_HOST")
	apiPort := os.Getenv("PORT")

	// sslmode defaults to disable because the DB is only reachable on the private
	// compose network; set DB_SSLMODE=require for a remote database.
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), config.Getenv("DB_SSLMODE", "disable"))

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	log.Println("Database connection established")

	derivedCatalog, err := models.LoadMetricsCatalog("metrics_catalog.yaml")
	if err != nil {
		log.Printf("Warning: metrics catalog not loaded: %v", err)
		derivedCatalog = models.DerivedMetricsCatalog{}
	} else {
		log.Printf("Metrics catalog loaded: %d derived metrics", len(derivedCatalog))
	}

	handler := handlers.NewHandler(db, derivedCatalog)

	// Surfaces the ldflags release version as OpenAPI info.version.
	docs.SwaggerInfo.Version = version.Version
	log.Printf("Version %s (build %s)", version.Version, version.BuildTime)

	// SWAGGER_HOST (public hostname behind the proxy) overrides API_HOST:PORT.
	if swaggerHost := config.Getenv("SWAGGER_HOST", ""); swaggerHost != "" {
		docs.SwaggerInfo.Host = swaggerHost
		docs.SwaggerInfo.Schemes = []string{"https"}
	} else {
		docs.SwaggerInfo.Host = fmt.Sprintf("%s:%s", apiHost, apiPort)
	}

	router := routes.SetupRoutes(handler)

	// Explicit timeouts: the default http.Server has none, which makes slowloris
	// and stuck connections trivial. WriteTimeout is generous for large CSV/JSON exports.
	srv := &http.Server{
		Addr:              ":" + apiPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	log.Printf("Server listening on %s:%s", apiHost, apiPort)
	log.Printf("Swagger UI: http://%s:%s/api/docs/index.html", apiHost, apiPort)

	// Graceful shutdown so in-flight requests finish on container stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed: %v", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Graceful shutdown failed: %v", err)
		}
	}
}

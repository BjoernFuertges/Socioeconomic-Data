package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/docs"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/config"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/handlers"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/routes"
)

// @title Sozioökonomische Daten API
// @version 1.0
// @description REST-API für LAU-Geodaten und Arbeitslosenzahlen aus dbt-transformierten Daten
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email Socioeconomic-Data@b.sgtrf.de

// @BasePath /api/v1
// @schemes http
func main() {
	// Lade Konfiguration aus .env Datei
	if err := config.Load(); err != nil {
		log.Fatalf("Fehler beim Laden der Konfiguration: %v", err)
	}

	// Server-Konfiguration aus Umgebungsvariablen
	apiHost := getEnv("API_HOST", "localhost")
	apiPort := getEnv("PORT", "8081")

	// Datenbankverbindung aus Umgebungsvariablen
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "sozio_data_compose_user")
	dbPassword := getEnv("DB_PASSWORD", "compose_password")
	dbName := getEnv("DB_NAME", "soziooekonomische-daten")

	// Connection String erstellen
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Datenbankverbindung mit sqlx öffnen
	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		log.Fatalf("Fehler beim Verbinden zur Datenbank: %v", err)
	}
	defer db.Close()

	// Connection Pool Einstellungen
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	log.Println("Datenbankverbindung erfolgreich hergestellt")

	// Metriken-Katalog aus YAML laden
	derivedCatalog, err := models.LoadMetricsCatalog("metrics_catalog.yaml")
	if err != nil {
		log.Printf("Warnung: Metriken-Katalog konnte nicht geladen werden: %v", err)
		derivedCatalog = models.DerivedMetricsCatalog{}
	} else {
		log.Printf("Metriken-Katalog geladen: %d abgeleitete Metriken", len(derivedCatalog))
	}

	// Handler mit Datenbankverbindung erstellen
	handler := handlers.NewHandler(db, derivedCatalog)

	// Swagger Host dynamisch setzen (SWAGGER_HOST überschreibt API_HOST:PORT)
	swaggerHost := getEnv("SWAGGER_HOST", fmt.Sprintf("%s:%s", apiHost, apiPort))
	docs.SwaggerInfo.Host = swaggerHost
	if getEnv("SWAGGER_HOST", "") != "" {
		docs.SwaggerInfo.Schemes = []string{"https"}
	}

	// Router mit allen Routen konfigurieren
	router := routes.SetupRoutes(handler)

	// Server starten
	log.Printf("Server läuft auf %s:%s", apiHost, apiPort)
	log.Printf("Swagger UI: http://%s:%s/api/docs/index.html", apiHost, apiPort)
	if err := router.Run(":" + apiPort); err != nil {
		log.Fatalf("Fehler beim Starten des Servers: %v", err)
	}
}

// getEnv liest Umgebungsvariable oder gibt Default-Wert zurück
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

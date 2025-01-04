package main

import (
	"github.com/Waelson/lock-manager-service/order-service-api/internal/db"
	"github.com/Waelson/lock-manager-service/order-service-api/internal/handler"
	"github.com/Waelson/lock-manager-service/order-service-api/internal/repository"
	"github.com/Waelson/lock-manager-service/order-service-api/pkg/sdk/locker"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	// Configurações do banco de dados obtidas das variáveis de ambiente
	dbConfig := db.Config{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     getEnvAsInt("POSTGRES_PORT", 5432),
		User:     getEnv("POSTGRES_USER", "postgres"),
		Password: getEnv("POSTGRES_PASSWORD", "password"),
		DBName:   getEnv("POSTGRES_DB", "inventory_db"),
	}

	// Conexão com o banco de dados
	conn, err := db.Connect(dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close()

	// Repositório de estoque
	inventoryRepo := repository.NewInventoryRepository(conn)

	// Instância do cliente de lock
	lockServiceUrl := getEnv("LOCK_SERVICE_URL", "http://localhost:8181")
	lockClient := locker.NewLockClient(lockServiceUrl)

	// Configuração do router
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Registro dos handlers
	r.Post("/order", handler.NewOrderHandler(inventoryRepo, lockClient))

	// Inicialização do servidor
	log.Println("Starting order-service-api on :9090...")
	if err := http.ListenAndServe(":9090", r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// getEnv retorna o valor da variável de ambiente ou um valor padrão
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsInt retorna o valor da variável de ambiente como int ou um valor padrão
func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

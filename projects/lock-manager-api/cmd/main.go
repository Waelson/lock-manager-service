package main

import (
	"fmt"
	"github.com/Waelson/lock-manager-service/lock-manager-api/internal/handler"
	"github.com/Waelson/lock-manager-service/lock-manager-api/internal/locker"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"net/http"
)

func main() {
	// Initiate Redis clients
	redisNodes := []*redis.Client{
		redis.NewClient(&redis.Options{Addr: "localhost:6379"}),
		redis.NewClient(&redis.Options{Addr: "localhost:6380"}),
		redis.NewClient(&redis.Options{Addr: "localhost:6381"}),
	}

	// Initiate locker
	redisLocker := locker.NewLocker(redisNodes)

	lockHandler := handler.NewLockHandler(redisLocker)

	// Set router
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Endpoints
	r.Post("/lock", lockHandler.AcquireLockHandler)
	r.Post("/unlock", lockHandler.ReleaseLockHandler)

	// Start web server
	fmt.Println("Server started at http://localhost:8181")
	if err := http.ListenAndServe(":8181", r); err != nil {
		panic(fmt.Sprintf("Erro ao iniciar o servidor: %v", err))
	}
}

package main

import (
	"errors"
	"fmt"
	"github.com/Waelson/lock-manager-service/lock-manager-api/internal/handler"
	"github.com/Waelson/lock-manager-service/lock-manager-api/internal/locker"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
)

func main() {
	redisAddresses := strings.TrimSpace(os.Getenv("REDIS_ADDRESSES"))

	// Initiate Redis clients
	redisNodes, err := CreateRedisClients(redisAddresses)
	if err != nil {
		panic(err)
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
	r.Post("/refresh", lockHandler.RefreshLockHandler)
	r.Get("/ttl", lockHandler.TTLHandler)

	// Print Redis and endpoint details
	PrintServerDetails(redisNodes)

	// Start web server
	fmt.Println("\nServer started at http://localhost:8181")
	if err := http.ListenAndServe(":8181", r); err != nil {
		panic(fmt.Sprintf("Error starting server: %v", err))
	}
}

// CreateRedisClients creates Redis clients from a comma-separated string of addresses
func CreateRedisClients(addresses string) ([]*redis.Client, error) {
	if strings.TrimSpace(addresses) == "" {
		return nil, errors.New("input string of Redis addresses is empty")
	}

	addrList := strings.Split(addresses, ",")
	if len(addrList) <= 2 {
		return nil, errors.New("number of Redis servers must be greater than 2")
	}
	if len(addrList)%2 == 0 {
		return nil, errors.New("number of Redis servers must be odd")
	}

	clients := make([]*redis.Client, 0, len(addrList))
	for _, addr := range addrList {
		client := redis.NewClient(&redis.Options{
			Addr: addr,
		})
		clients = append(clients, client)
	}

	return clients, nil
}

// PrintServerDetails prints Redis servers and endpoints in a professional table format
func PrintServerDetails(redisNodes []*redis.Client) {
	fmt.Println("\n==========================")
	fmt.Println("   REDIS SERVER DETAILS   ")
	fmt.Println("==========================")

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(writer, "SERVER ID\tADDRESS\tSTATUS")
	fmt.Fprintln(writer, "---------\t-------\t------")

	for i, node := range redisNodes {
		// Simulating status for demonstration (you can replace this with actual health checks)
		status := "UP"
		fmt.Fprintf(writer, "Server %d\t%s\t%s\n", i+1, node.Options().Addr, status)
	}
	writer.Flush()

	fmt.Println("\n=========================")
	fmt.Println("      API ENDPOINTS      ")
	fmt.Println("=========================")

	writer = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(writer, "ENDPOINT\tMETHOD")
	fmt.Fprintln(writer, "--------\t------")
	fmt.Fprintln(writer, "/lock\tPOST")
	fmt.Fprintln(writer, "/unlock\tPOST")
	fmt.Fprintln(writer, "/refresh\tPOST")
	fmt.Fprintln(writer, "/ttl\tGET")
	writer.Flush()

	fmt.Println("\n=========================")
}

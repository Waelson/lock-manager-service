package handler

import (
	"context"
	"encoding/json"
	"github.com/Waelson/lock-manager-service/order-service-api/internal/repository"
	"github.com/Waelson/lock-manager-service/order-service-api/pkg/sdk/locker"
	"net/http"
	"time"
)

type OrderRequest struct {
	ItemName string `json:"item_name"`
	Quantity int    `json:"quantity"`
}

type OrderResponse struct {
	Message string `json:"message"`
}

// NewOrderHandler cria um handler para o endpoint /order
func NewOrderHandler(repo *repository.InventoryRepository, lockClient *locker.LockClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req OrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		ctx, cancelFunc := context.WithTimeout(r.Context(), 200*time.Millisecond)
		defer cancelFunc()

		// Adquire o lock para o item
		lock, releaseFunc, err := lockClient.Acquire(ctx, req.ItemName, "50ms", "100ms")
		if err != nil {
			http.Error(w, "Failed to acquire lock", http.StatusConflict)
			return
		}

		//Vamos garantir que o lock seja sempre liberado
		defer releaseFunc()

		// Verifica a quantidade disponível
		availableQuantity, err := repo.GetAvailableQuantity(ctx, req.ItemName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Verifica se a quantidade solicitada está disponível
		if availableQuantity < req.Quantity {
			http.Error(w, "Insufficient quantity available", http.StatusConflict)
			return
		}

		// Atualiza a quantidade no banco de dados
		if err := repo.DecrementQuantity(ctx, req.ItemName, req.Quantity); err != nil {
			http.Error(w, "Failed to update inventory", http.StatusInternalServerError)
			return
		}

		lockClient.Release(ctx, lock)

		// Retorna resposta de sucesso
		res := OrderResponse{
			Message: "Order successfully placed",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}

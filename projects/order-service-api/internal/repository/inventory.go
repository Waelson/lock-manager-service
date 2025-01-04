package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// InventoryRepository representa o repositório para manipulação do estoque
type InventoryRepository struct {
	db *sql.DB
}

// NewInventoryRepository cria uma nova instância do repositório
func NewInventoryRepository(db *sql.DB) *InventoryRepository {
	return &InventoryRepository{db: db}
}

// GetAvailableQuantity consulta a quantidade disponível de um item no estoque
func (r *InventoryRepository) GetAvailableQuantity(ctx context.Context, itemName string) (int, error) {
	var quantity int
	err := r.db.QueryRowContext(ctx, "SELECT quantity FROM tb_inventory WHERE item_name = $1", itemName).Scan(&quantity)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("item '%s' not found", itemName)
	} else if err != nil {
		return 0, err
	}
	return quantity, nil
}

// DecrementQuantity decrementa a quantidade de um item no estoque
func (r *InventoryRepository) DecrementQuantity(ctx context.Context, itemName string, quantity int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE tb_inventory SET quantity = quantity - $1 WHERE item_name = $2", quantity, itemName)
	return err
}

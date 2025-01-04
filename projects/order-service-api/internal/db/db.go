package db

import (
	"database/sql"
	"fmt"
)

// Config representa a configuração para conexão ao banco de dados
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

// Connect realiza a conexão com o banco de dados PostgreSQL
func Connect(cfg Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)
	return sql.Open("postgres", dsn)
}

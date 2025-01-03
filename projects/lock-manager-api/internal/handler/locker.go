package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Waelson/lock-manager-service/lock-manager-api/internal/locker"
	"golang.org/x/net/context"
	"net/http"
	"time"
)

type AcquireLockResponse struct {
	Code     int    `json:"code,omitempty"`
	Token    string `json:"token,omitempty"`
	Resource string `json:"resource,omitempty"`
	Ttl      string `json:"ttl,omitempty"`
	Acquired bool   `json:"acquired"`
	Message  string `json:"message,omitempty"`
}

type ReleaseLockResponse struct {
	Code     int    `json:"code"`
	Token    string `json:"token"`
	Resource string `json:"resource"`
}

type lockerHandler struct {
	redlock locker.RedLocker
}

type LockerHandler interface {
	AcquireLockHandler(w http.ResponseWriter, r *http.Request)
	ReleaseLockHandler(w http.ResponseWriter, r *http.Request)
}

func NewLockHandler(redlock locker.RedLocker) LockerHandler {
	return &lockerHandler{redlock: redlock}
}

func (l *lockerHandler) AcquireLockHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resource := r.URL.Query().Get("resource")
	if resource == "" {
		l.jsonError(w, "Faltando parâmetro 'resource'", http.StatusBadRequest)
		return
	}

	ttl := r.URL.Query().Get("ttl")
	if ttl == "" {
		ttl = "10ms"
	} else {
		ttl = fmt.Sprintf("%sms", ttl)
	}

	duration, err := time.ParseDuration(ttl)
	if err != nil {
		l.jsonError(w, "Valor inválido para 'ttl'", http.StatusBadRequest)
		return
	}

	lock, err := l.redlock.Acquire(ctx, resource, duration)
	if err != nil {
		if errors.Is(err, locker.AcquireLockError) {
			l.jsonResponse(w, AcquireLockResponse{
				Code:     http.StatusConflict,
				Resource: resource,
				Message:  err.Error(),
				Acquired: false,
			}, http.StatusConflict)
		} else {
			l.jsonError(w, "Erro interno ao adquirir o lock", http.StatusInternalServerError)
		}
		return
	}

	l.jsonResponse(w, AcquireLockResponse{
		Code:     http.StatusOK,
		Token:    lock.Token,
		Resource: lock.Resource,
		Ttl:      ttl,
		Acquired: true,
	}, http.StatusOK)
}

func (l *lockerHandler) ReleaseLockHandler(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		l.jsonError(w, "Faltando parâmetro 'resource'", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		l.jsonError(w, "Faltando parâmetro 'token'", http.StatusBadRequest)
		return
	}

	err := l.redlock.Release(context.Background(), resource, token)
	if err != nil {
		l.jsonError(w, fmt.Sprintf("Erro ao liberar o lock: %v", err), http.StatusInternalServerError)
		return
	}

	l.jsonResponse(w, ReleaseLockResponse{
		Code:     http.StatusOK,
		Token:    token,
		Resource: resource,
	}, http.StatusOK)
}

func (l *lockerHandler) jsonResponse(w http.ResponseWriter, content interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(content); err != nil {
		http.Error(w, "Erro ao converter resposta em JSON", http.StatusInternalServerError)
	}
}

// Função auxiliar para responder erros JSON
func (l *lockerHandler) jsonError(w http.ResponseWriter, message string, code int) {
	l.jsonResponse(w, map[string]string{"error": message}, code)
}

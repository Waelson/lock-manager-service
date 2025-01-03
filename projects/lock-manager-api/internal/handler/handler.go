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

type RefreshLockResponse struct {
	Code      int    `json:"code"`
	Token     string `json:"token"`
	Resource  string `json:"resource"`
	Ttl       string `json:"ttl"`
	Refreshed bool   `json:"refreshed"`
	Message   string `json:"message,omitempty"`
}

type TTLResponse struct {
	Code     int    `json:"code"`
	Resource string `json:"resource"`
	Token    string `json:"token"`
	Ttl      string `json:"ttl"`
	Message  string `json:"message,omitempty"`
}

type lockerHandler struct {
	redlock locker.RedLocker
}

type LockerHandler interface {
	AcquireLockHandler(w http.ResponseWriter, r *http.Request)
	ReleaseLockHandler(w http.ResponseWriter, r *http.Request)
	RefreshLockHandler(w http.ResponseWriter, r *http.Request)
	TTLHandler(w http.ResponseWriter, r *http.Request)
}

func (l *lockerHandler) TTLHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Obtém os parâmetros da requisição
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		l.jsonError(w, "missing 'resource' parameter", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		l.jsonError(w, "missing 'token' parameter", http.StatusBadRequest)
		return
	}

	// Verifica o tempo restante do lock
	ttl, err := l.redlock.TTL(ctx, resource, token)
	if err != nil {
		if errors.Is(err, locker.LockNotFoundError) {
			l.jsonResponse(w, TTLResponse{
				Code:     http.StatusNotFound,
				Resource: resource,
				Token:    token,
				Ttl:      "0s",
				Message:  "lock not found or expired",
			}, http.StatusNotFound)
		} else {
			l.jsonError(w, "internal error while checking TTL", http.StatusInternalServerError)
		}
		return
	}

	// Responde com sucesso
	l.jsonResponse(w, TTLResponse{
		Code:     http.StatusOK,
		Resource: resource,
		Token:    token,
		Ttl:      ttl.String(),
	}, http.StatusOK)
}

func NewLockHandler(redlock locker.RedLocker) LockerHandler {
	return &lockerHandler{redlock: redlock}
}

func (l *lockerHandler) RefreshLockHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Obtém os parâmetros da requisição
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		l.jsonError(w, "missing 'resource' parameter", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		l.jsonError(w, "missing 'token' parameter", http.StatusBadRequest)
		return
	}

	ttl := r.URL.Query().Get("ttl")
	if ttl == "" {
		ttl = "10s" // TTL padrão
	}

	duration, err := time.ParseDuration(ttl)
	if err != nil {
		l.jsonError(w, "invalid 'ttl' value", http.StatusBadRequest)
		return
	}

	// Tenta atualizar o lock
	err = l.redlock.Refresh(ctx, resource, token, duration)
	if err != nil {
		if errors.Is(err, locker.LockNotFoundError) {
			l.jsonResponse(w, RefreshLockResponse{
				Code:      http.StatusNotFound,
				Resource:  resource,
				Token:     token,
				Ttl:       ttl,
				Refreshed: false,
				Message:   err.Error(),
			}, http.StatusNotFound)
		} else {
			l.jsonError(w, "internal error while refreshing lock", http.StatusInternalServerError)
		}
		return
	}

	// Responde com sucesso
	l.jsonResponse(w, RefreshLockResponse{
		Code:      http.StatusOK,
		Token:     token,
		Resource:  resource,
		Ttl:       ttl,
		Refreshed: true,
	}, http.StatusOK)
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
		l.jsonError(w, "missing 'resource' parameter", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		l.jsonError(w, "missing 'token' parameter", http.StatusBadRequest)
		return
	}

	err := l.redlock.Release(context.Background(), resource, token)
	if err != nil {
		if errors.Is(err, locker.LockNotFoundError) {
			l.jsonResponse(w, map[string]interface{}{
				"code":     http.StatusNotFound,
				"resource": resource,
				"token":    token,
				"message":  "lock not found or expired",
			}, http.StatusNotFound)
			return
		} else if errors.Is(err, locker.InternalError) {
			l.jsonError(w, "internal error while releasing lock", http.StatusInternalServerError)
			return
		} else {
			l.jsonError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
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

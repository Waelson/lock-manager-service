package locker

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/context"
	"sync"
	"time"
)

var (
	AcquireLockError = errors.New("lock already acquired")
	InternalError    = errors.New("error connecting to node")
)

type Locker struct {
	Ttl      int64
	Token    string
	Resource string
}

type redLock struct {
	redisNodes []*redis.Client
	quorum     int
}

type RedLocker interface {
	Acquire(ctx context.Context, resource string, ttl time.Duration) (*Locker, error)
	Release(ctx context.Context, resource string, token string) error
}

func (l *redLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (*Locker, error) {
	token := uuid.New().String()
	lockCount := 0
	startTime := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, len(l.redisNodes))

	// Paraleliza a tentativa de aquisição do lock em cada nó Redis
	for _, node := range l.redisNodes {
		wg.Add(1)
		go func(node *redis.Client) {
			defer wg.Done()
			ok, err := node.SetNX(ctx, resource, token, ttl).Result()
			if err != nil {
				errChan <- fmt.Errorf("error on node %v: %w", node.Options().Addr, err)
				return
			}
			if ok {
				mu.Lock()
				lockCount++
				mu.Unlock()
			}
		}(node)
	}

	// Aguarda a conclusão das operações
	wg.Wait()
	close(errChan)

	// Verifica erros
	for err := range errChan {
		fmt.Println(err.Error())
		return nil, InternalError
	}

	// Verifica se o quórum foi atingido e o tempo total não excedeu o TTL
	elapsed := time.Since(startTime)
	if lockCount >= l.quorum && elapsed < ttl {
		return &Locker{
			Ttl:      ttl.Milliseconds(),
			Token:    token,
			Resource: resource,
		}, nil
	}

	// Falha ao adquirir o lock, libera qualquer lock parcial
	_ = l.Release(ctx, resource, token)
	return nil, AcquireLockError
}

// Release libera o lock em todas as instâncias Redis
func (l *redLock) Release(ctx context.Context, resource string, token string) error {
	var wg sync.WaitGroup

	// Paraleliza a liberação do lock em cada nó Redis
	for _, node := range l.redisNodes {
		wg.Add(1)
		go func(node *redis.Client) {
			defer wg.Done()
			val, err := node.Get(ctx, resource).Result()
			if errors.Is(err, redis.Nil) {
				return // A chave não existe
			} else if err != nil {
				fmt.Printf("Erro ao tentar liberar lock no nó %v: %v\n", node.Options().Addr, err)
				return
			}

			// Verifica se o lock pertence ao cliente
			if val == token {
				_, err := node.Del(ctx, resource).Result()
				fmt.Printf("Liberando recurso/token '%s/%s' no nó %s\n", resource, token, node.String())
				if err != nil {
					fmt.Printf("Erro ao deletar chave no nó %v: %v\n", node.Options().Addr, err)
				}
			}
		}(node)
	}

	wg.Wait()
	return nil
}

func NewLocker(redisNodes []*redis.Client) RedLocker {
	quorum := len(redisNodes)/2 + 1
	return &redLock{
		redisNodes: redisNodes,
		quorum:     quorum,
	}
}

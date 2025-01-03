package locker

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/context"
	"log"
	"sync"
	"time"
)

var (
	AcquireLockError  = errors.New("lock already acquired")
	LockNotFoundError = errors.New("lock not found or expired")
	InternalError     = errors.New("error connecting to one or more nodes")
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
	Refresh(ctx context.Context, resource string, token string, ttl time.Duration) error
	TTL(ctx context.Context, resource string, token string) (time.Duration, error)
}

// TTL checks the remaining time-to-live (TTL) of a lock
func (l *redLock) TTL(ctx context.Context, resource string, token string) (time.Duration, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	ttlCount := 0
	totalTTL := int64(0)
	errs := make([]error, 0)

	// Parallelize the TTL check operation on each Redis node
	for _, node := range l.redisNodes {
		wg.Add(1)
		go func(node *redis.Client) {
			defer wg.Done()

			nodeCtx, cancel := context.WithTimeout(ctx, 2*time.Second) // Timeout per node
			defer cancel()

			val, err := node.Get(nodeCtx, resource).Result()
			if errors.Is(err, redis.Nil) {
				return // Key does not exist
			} else if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("error checking lock on node %v: %w", node.Options().Addr, err))
				mu.Unlock()
				return
			}

			// Verify if the lock belongs to the client
			if val == token {
				ttl, err := node.TTL(nodeCtx, resource).Result()
				if err == nil && ttl > 0 {
					mu.Lock()
					totalTTL += int64(ttl.Seconds())
					log.Printf("get TTL from resource '%s#%s' on node %s\n", resource, token, node.String())
					ttlCount++
					mu.Unlock()
				} else if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("error getting TTL on node %v: %w", node.Options().Addr, err))
					mu.Unlock()
				}
			}
		}(node)
	}

	wg.Wait()

	// Log errors if any
	if len(errs) > 0 {
		log.Printf("errors while getting TTL: %v\n", errs)
	}

	// Check if quorum was reached
	if ttlCount >= l.quorum {
		// Return the average TTL across nodes in the quorum
		avgTTL := time.Duration(totalTTL/int64(ttlCount)) * time.Second
		return avgTTL, nil
	}

	return 0, LockNotFoundError
}

// Acquire attempts to acquire the lock across multiple Redis nodes
func (l *redLock) Acquire(ctx context.Context, resource string, ttl time.Duration) (*Locker, error) {
	token := uuid.New().String()
	lockCount := 0
	startTime := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0)
	errChan := make(chan error, len(l.redisNodes))

	// Parallelize the lock acquisition attempt on each Redis node
	for _, node := range l.redisNodes {
		wg.Add(1)
		go func(node *redis.Client) {
			defer wg.Done()

			nodeCtx, cancel := context.WithTimeout(ctx, 2*time.Second) // Timeout per node
			defer cancel()

			ok, err := node.SetNX(nodeCtx, resource, token, ttl).Result()
			if err != nil {
				errChan <- fmt.Errorf("error on node %v: %w", node.Options().Addr, err)
				return
			}
			if ok {
				mu.Lock()
				lockCount++
				log.Printf("resource '%s#%s' locked on node %s\n", resource, token, node.String())
				mu.Unlock()
			}
		}(node)
	}

	// Wait for all attempts to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	for err := range errChan {
		errs = append(errs, err)
	}

	// Log errors if any
	if len(errs) > 0 {
		log.Printf("errors while acquiring lock: %v\n", errs)
	}

	// Check if quorum was reached and TTL is still valid
	elapsed := time.Since(startTime)
	if lockCount >= l.quorum && elapsed < ttl {
		return &Locker{
			Ttl:      ttl.Milliseconds(),
			Token:    token,
			Resource: resource,
		}, nil
	}

	// Release partial locks on failure
	_ = l.Release(ctx, resource, token)
	return nil, AcquireLockError
}

// Release releases the lock on all Redis nodes
func (l *redLock) Release(ctx context.Context, resource string, token string) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	notFoundCount := 0
	errs := make([]error, 0)

	// Parallelize the lock release on each Redis node
	for _, node := range l.redisNodes {
		wg.Add(1)
		go func(node *redis.Client) {
			defer wg.Done()

			nodeCtx, cancel := context.WithTimeout(ctx, 2*time.Second) // Timeout per node
			defer cancel()

			val, err := node.Get(nodeCtx, resource).Result()
			if errors.Is(err, redis.Nil) {
				mu.Lock()
				notFoundCount++
				mu.Unlock()
				return // Key does not exist
			} else if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("error on node %v: %w", node.Options().Addr, err))
				mu.Unlock()
				return
			}

			// Verify if the lock belongs to the client
			if val == token {
				_, err := node.Del(nodeCtx, resource).Result()
				if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("error deleting key on node %v: %w", node.Options().Addr, err))
					mu.Unlock()
				} else {
					log.Printf("resource '%s#%s' released on node %s\n", resource, token, node.String())
				}
			} else {
				mu.Lock()
				errs = append(errs, fmt.Errorf("lock mismatch on node %v: token does not match", node.Options().Addr))
				mu.Unlock()
			}
		}(node)
	}

	wg.Wait()

	// Log errors if any
	if len(errs) > 0 {
		log.Printf("errors while releasing lock: %v\n", errs)
	}

	// Check if quorum indicates the lock was not found
	if notFoundCount >= l.quorum {
		return LockNotFoundError
	}

	// If there are other errors but the lock was released successfully on some nodes, return a generic error
	if len(errs) > 0 {
		return InternalError
	}

	return nil
}

// Refresh verifies if the lock is active and extends its TTL
func (l *redLock) Refresh(ctx context.Context, resource string, token string, ttl time.Duration) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	activeCount := 0
	errs := make([]error, 0)

	// Parallelize the refresh operation on each Redis node
	for _, node := range l.redisNodes {
		wg.Add(1)
		go func(node *redis.Client) {
			defer wg.Done()

			nodeCtx, cancel := context.WithTimeout(ctx, 2*time.Second) // Timeout per node
			defer cancel()

			val, err := node.Get(nodeCtx, resource).Result()
			if errors.Is(err, redis.Nil) {
				return // Key does not exist
			} else if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("error checking lock on node %v: %w", node.Options().Addr, err))
				mu.Unlock()
				return
			}

			// Verify if the lock belongs to the client
			if val == token {
				_, err := node.Expire(nodeCtx, resource, ttl).Result()
				if err == nil {
					mu.Lock()
					activeCount++
					log.Printf("resource '%s#%s' refreshed on node %s\n", resource, token, node.String())
					mu.Unlock()
				} else {
					mu.Lock()
					errs = append(errs, fmt.Errorf("error refreshing lock on node %v: %w", node.Options().Addr, err))
					mu.Unlock()
				}
			}
		}(node)
	}

	wg.Wait()

	// Log errors if any
	if len(errs) > 0 {
		log.Printf("errors while refreshing lock: %v\n", errs)
	}

	// Check if quorum was reached
	if activeCount >= l.quorum {
		return nil
	}

	return LockNotFoundError
}

// NewLocker creates a new RedLocker instance
func NewLocker(redisNodes []*redis.Client) RedLocker {
	quorum := len(redisNodes)/2 + 1
	return &redLock{
		redisNodes: redisNodes,
		quorum:     quorum,
	}
}

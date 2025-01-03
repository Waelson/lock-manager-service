package locker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

var (
	ErrLockConflict    = errors.New("lock already acquired (HTTP 409)")
	ErrTimeout         = errors.New("operation timed out")
	ErrServerError     = errors.New("internal server error")
	ErrReleaseNotFound = errors.New("lock not found or already released (HTTP 404)")
)

type Lock struct {
	Token     string
	Resource  string
	StartTime time.Time
}

func newLock(token string, resource string) *Lock {
	return &Lock{
		Token:     token,
		Resource:  resource,
		StartTime: time.Now(),
	}
}

func (l *Lock) String() string {
	return fmt.Sprintf("Token: %s Resource: %s StartTime: %s", l.Token, l.Resource, l.StartTime.String())
}

// ExponentialBackoff represents the configuration for exponential backoff with jitter
type ExponentialBackoff struct {
	Initial   time.Duration // Initial backoff duration
	Max       time.Duration // Maximum backoff duration
	MaxJitter time.Duration // Maximum jitter duration
}

// LockClient represents the SDK for interacting with the lock service
type LockClient struct {
	baseURL       string
	httpClient    *http.Client
	backoffConfig *ExponentialBackoff
}

// Option defines a functional option for LockClient
type Option func(*LockClient)

// WithExponentialBackoff sets the exponential backoff configuration for LockClient
func WithExponentialBackoff(backoff *ExponentialBackoff) Option {
	return func(sdk *LockClient) {
		sdk.backoffConfig = backoff
	}
}

// NewLockClient initializes a new instance of LockClient with optional functional options
func NewLockClient(baseURL string, opts ...Option) *LockClient {
	sdk := &LockClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	for _, opt := range opts {
		opt(sdk)
	}

	// Set default backoff if not provided
	if sdk.backoffConfig == nil {
		sdk.backoffConfig = &ExponentialBackoff{
			Initial:   100 * time.Millisecond,
			Max:       5 * time.Second,
			MaxJitter: 500 * time.Millisecond,
		}
	}

	return sdk
}

// Acquire tries to acquire a lock, retrying if the API returns HTTP 409, within the "expire" duration.
// Returns the token and a release function.
func (sdk *LockClient) Acquire(ctx context.Context, resource string, ttl string, expire string) (*Lock, func() error, error) {
	if resource == "" {
		return nil, nil, errors.New("resource must not be empty")
	}

	ttlDuration, err := time.ParseDuration(ttl)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid TTL value: %w", err)
	}

	expireDuration, err := time.ParseDuration(expire)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid expire value: %w", err)
	}

	endTime := time.Now().Add(expireDuration)
	backoff := sdk.backoffConfig.Initial

	var token string

	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		token, err = sdk.tryAcquire(ctx, resource, ttlDuration)
		if err == nil {
			break
		}

		if !errors.Is(err, ErrLockConflict) {
			return nil, nil, err
		}

		// Check if we are out of time
		if time.Now().After(endTime) {
			return nil, nil, ErrTimeout
		}

		// Apply exponential backoff with jitter
		backoff = sdk.calculateBackoff(backoff)
		time.Sleep(backoff)
	}

	lock := newLock(token, resource)

	// Release function
	releaseFunc := func() error {
		return sdk.Release(ctx, lock)
	}

	return lock, releaseFunc, nil
}

func (sdk *LockClient) calculateBackoff(currentBackoff time.Duration) time.Duration {
	nextBackoff := currentBackoff * 2
	if nextBackoff > sdk.backoffConfig.Max {
		nextBackoff = sdk.backoffConfig.Max
	}

	// Add jitter
	jitter := time.Duration(rand.Int63n(int64(sdk.backoffConfig.MaxJitter)))
	return nextBackoff + jitter
}

func (sdk *LockClient) tryAcquire(ctx context.Context, resource string, ttl time.Duration) (string, error) {
	url := fmt.Sprintf("%s/lock", sdk.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	query := req.URL.Query()
	query.Add("resource", resource)
	query.Add("ttl", ttl.String())
	req.URL.RawQuery = query.Encode()

	resp, err := sdk.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return "", ErrLockConflict
	}

	if resp.StatusCode != http.StatusOK {
		return "", ErrServerError
	}

	var res struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if res.Token == "" {
		return "", errors.New("no token returned from server")
	}

	return res.Token, nil
}

// Release releases a lock associated with the given resource and token
func (sdk *LockClient) Release(ctx context.Context, lock *Lock) error {
	if lock.Resource == "" {
		return errors.New("resource must not be empty")
	}
	if lock.Resource == "" {
		return errors.New("token must not be empty")
	}

	url := fmt.Sprintf("%s/unlock", sdk.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	query := req.URL.Query()
	query.Add("resource", lock.Resource)
	query.Add("token", lock.Token)
	req.URL.RawQuery = query.Encode()

	resp, err := sdk.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrReleaseNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to release lock: HTTP %d", resp.StatusCode)
	}

	// Optional: Decode response for additional logging or validation
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if res.Code != http.StatusOK {
		return fmt.Errorf("unexpected response code: %d, message: %s", res.Code, res.Message)
	}

	return nil
}

// Refresh extends the TTL of a lock to keep it active
func (sdk *LockClient) Refresh(ctx context.Context, lock *Lock, ttl string) error {
	if lock.Resource == "" {
		return errors.New("resource must not be empty")
	}
	if lock.Token == "" {
		return errors.New("token must not be empty")
	}

	ttlDuration, err := time.ParseDuration(ttl)
	if err != nil {
		return fmt.Errorf("invalid TTL value: %w", err)
	}

	url := fmt.Sprintf("%s/refresh", sdk.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	query := req.URL.Query()
	query.Add("resource", lock.Resource)
	query.Add("token", lock.Token)
	query.Add("ttl", ttlDuration.String())
	req.URL.RawQuery = query.Encode()

	resp, err := sdk.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrReleaseNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to refresh lock: HTTP %d", resp.StatusCode)
	}

	// Optional: Decode response for logging or validation
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if res.Code != http.StatusOK {
		return fmt.Errorf("unexpected response code: %d, message: %s", res.Code, res.Message)
	}

	// Update lock start time after refresh
	lock.StartTime = time.Now()

	return nil
}

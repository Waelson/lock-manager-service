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

// ExponentialBackoff represents the configuration for exponential backoff with jitter
type ExponentialBackoff struct {
	Initial   time.Duration // Initial backoff duration
	Max       time.Duration // Maximum backoff duration
	MaxJitter time.Duration // Maximum jitter duration
}

// LockSDK represents the SDK for interacting with the lock service
type LockSDK struct {
	BaseURL       string
	HTTPClient    *http.Client
	BackoffConfig *ExponentialBackoff
}

// Option defines a functional option for LockSDK
type Option func(*LockSDK)

// WithExponentialBackoff sets the exponential backoff configuration for LockSDK
func WithExponentialBackoff(backoff *ExponentialBackoff) Option {
	return func(sdk *LockSDK) {
		sdk.BackoffConfig = backoff
	}
}

// NewLockSDK initializes a new instance of LockSDK with optional functional options
func NewLockSDK(baseURL string, opts ...Option) *LockSDK {
	sdk := &LockSDK{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	for _, opt := range opts {
		opt(sdk)
	}

	// Set default backoff if not provided
	if sdk.BackoffConfig == nil {
		sdk.BackoffConfig = &ExponentialBackoff{
			Initial:   100 * time.Millisecond,
			Max:       5 * time.Second,
			MaxJitter: 500 * time.Millisecond,
		}
	}

	return sdk
}

// Acquire tries to acquire a lock, retrying if the API returns HTTP 409, within the "expire" duration
func (sdk *LockSDK) Acquire(ctx context.Context, resource string, ttl string, expire string) (string, error) {
	if resource == "" {
		return "", errors.New("resource must not be empty")
	}

	ttlDuration, err := time.ParseDuration(ttl)
	if err != nil {
		return "", fmt.Errorf("invalid TTL value: %w", err)
	}

	expireDuration, err := time.ParseDuration(expire)
	if err != nil {
		return "", fmt.Errorf("invalid expire value: %w", err)
	}

	endTime := time.Now().Add(expireDuration)
	backoff := sdk.BackoffConfig.Initial

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		token, err := sdk.tryAcquire(ctx, resource, ttlDuration)
		if err == nil {
			return token, nil
		}

		if !errors.Is(err, ErrLockConflict) {
			return "", err
		}

		// Check if we are out of time
		if time.Now().After(endTime) {
			return "", ErrTimeout
		}

		// Apply exponential backoff with jitter
		backoff = sdk.calculateBackoff(backoff)
		time.Sleep(backoff)
	}
}

func (sdk *LockSDK) calculateBackoff(currentBackoff time.Duration) time.Duration {
	nextBackoff := currentBackoff * 2
	if nextBackoff > sdk.BackoffConfig.Max {
		nextBackoff = sdk.BackoffConfig.Max
	}

	// Add jitter
	jitter := time.Duration(rand.Int63n(int64(sdk.BackoffConfig.MaxJitter)))
	return nextBackoff + jitter
}

func (sdk *LockSDK) tryAcquire(ctx context.Context, resource string, ttl time.Duration) (string, error) {
	url := fmt.Sprintf("%s/lock", sdk.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	query := req.URL.Query()
	query.Add("resource", resource)
	query.Add("ttl", ttl.String())
	req.URL.RawQuery = query.Encode()

	resp, err := sdk.HTTPClient.Do(req)
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
func (sdk *LockSDK) Release(ctx context.Context, resource string, token string) error {
	if resource == "" {
		return errors.New("resource must not be empty")
	}
	if token == "" {
		return errors.New("token must not be empty")
	}

	url := fmt.Sprintf("%s/unlock", sdk.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	query := req.URL.Query()
	query.Add("resource", resource)
	query.Add("token", token)
	req.URL.RawQuery = query.Encode()

	resp, err := sdk.HTTPClient.Do(req)
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

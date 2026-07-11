package api

import "fmt"

// APIError represents an error response from the server.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// NotFoundError indicates the requested resource was not found.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// AuthError indicates an authentication or authorization failure.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// TimeoutError indicates a command timed out.
type TimeoutError struct {
	Message string
}

func (e *TimeoutError) Error() string {
	return e.Message
}

// ConnectionError indicates the server is unreachable.
type ConnectionError struct {
	Message string
}

func (e *ConnectionError) Error() string {
	return e.Message
}

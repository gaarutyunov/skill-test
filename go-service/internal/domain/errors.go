package domain

import "errors"

// Sentinel errors expressing business-level failure modes. Adapters translate
// transport/library errors into these; the transport layer maps them to HTTP
// status codes.
var (
	// ErrStudentNotFound indicates the requested student does not exist upstream.
	ErrStudentNotFound = errors.New("student not found")
	// ErrUnauthorized indicates the service could not authenticate upstream.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrUpstreamUnavailable indicates the upstream backend could not be reached
	// or returned an unexpected error.
	ErrUpstreamUnavailable = errors.New("upstream unavailable")
)

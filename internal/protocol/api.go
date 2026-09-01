package protocol

// APIError is the structured error payload returned by HTTP handlers.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// ErrorResponse wraps structured API errors.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

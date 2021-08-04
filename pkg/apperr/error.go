package apperr

type Error struct {
	Code    int    `json:"Code"`
	Status  string `json:"Status"`
	Message string `json:"Message"`
}

func (e *Error) Error() string {
	return e.Message
}

func BadRequest(m string) *Error {
	return &Error{Status: "bad-request", Code: 400, Message: m}
}

func EntityNotFound(m string) *Error {
	return &Error{Status: "entity-not-found", Code: 404, Message: m}
}

func UnsupportedMediaType(m string) *Error {
	return &Error{Status: "unsupported-media-type", Code: 415, Message: m}
}

func Unexpected(m string) *Error {
	return &Error{Status: "internal-server", Code: 500, Message: m}
}

// HomePage func
func RateLimit() *Error {
	return &Error{Status: "internal-server", Code: 429, Message: "Too Many requests!"}
}

type ValidationErrorResponse struct {
	Errors []string `json:"errors"`
}

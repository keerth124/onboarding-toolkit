package core

import (
	"fmt"
	"net/http"
	"strings"
)

func conjurHTTPError(context string, status int, body []byte, hint string) error {
	message := fmt.Sprintf("%s returned HTTP %d %s; response: %s", context, status, http.StatusText(status), responseDetail(body))
	if hint != "" {
		message += "; hint: " + hint
	}
	return fmt.Errorf("%s", message)
}

func responseDetail(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "<empty response>"
	}
	const limit = 2000
	if len(text) > limit {
		return text[:limit] + "...<truncated>"
	}
	return text
}

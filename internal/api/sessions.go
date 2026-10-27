package api

import (
	"net/http"

	"github.com/google/uuid"
)

func createSessionCookie(w http.ResponseWriter) uuid.UUID {
	sessionID := uuid.New()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID.String(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
		MaxAge:   7 * 24 * 60 * 60,
	})

	return sessionID
}

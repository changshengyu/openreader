package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"openreader/backend/services/authsession"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 5 * time.Second,
}

func (s *Server) syncSocket(c *gin.Context) {
	token := c.Query("token")
	identity, err := s.sessions.Authenticate(c.Request.Context(), token)
	if errors.Is(err, authsession.ErrInvalidSession) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate sync connection"})
		return
	}
	userID := identity.UserID

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := s.hub.AddClient(userID, conn)
	go client.WritePump()
	client.ReadPump()
}

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 5 * time.Second,
}

func (s *Server) syncSocket(c *gin.Context) {
	token := c.Query("token")
	userID, err := middleware.ParseToken(s.cfg.JWTSecret, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	var user models.User
	if err := s.db.Select("id").First(&user, userID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate sync connection"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := s.hub.AddClient(userID, conn)
	go client.WritePump()
	client.ReadPump()
}

package api

import (
	"github.com/gin-gonic/gin"
	db "github.com/stuneak/bagger/db/sqlc"
)

type Server struct {
	store  *db.Queries
	router *gin.Engine
}

func NewServer(store *db.Queries, ginMode string) *Server {
	server := &Server{store: store}
	router := gin.Default()

	gin.SetMode(ginMode)

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Routes
	router.GET("/mentions/:username", server.getUserMentions)

	server.router = router
	return server
}

func (server *Server) Start(address string) error {
	return server.router.Run(address)
}

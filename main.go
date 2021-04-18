package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/routes"
)

var globalClient *http.Client

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.LoadHTMLGlob("templates/*.tmpl.html")
	router.Static("/static", "static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", nil)
	})

	router.GET("/spotify/callback", callbackClientInjector)

	router.POST("/slack/events", eventsClientInjector)
	router.POST("/slack/interactivity", interactionsClientInjector)

	// Create the global spotify client
	globalClient = http.DefaultClient

	// Database setup
	database.ConnectToDatabase()
	database.ValidateSchema()

	router.Run(":" + port)
}

func callbackClientInjector(context *gin.Context) {
	routes.CallbackFlow(context, globalClient)
}

func eventsClientInjector(context *gin.Context) {
	routes.EventsEndpoint(context, globalClient)
}

func interactionsClientInjector(context *gin.Context) {
	routes.InteractivityEndpoint(context, globalClient)
}

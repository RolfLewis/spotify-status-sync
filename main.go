package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/routes"
	"rolflewis.com/spotify-status-sync/src/spotify"
)

var globalClient *http.Client

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	// Create the global spotify client
	globalClient = http.DefaultClient

	// Create routes
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

	// Database setup
	database.ConnectToDatabase()
	database.ValidateSchema()

	// Kick off the spotify token maintenance routine
	go spotifyTokenMaintenance()

	// Stand up server
	router.Run(":" + port)
}

func spotifyTokenMaintenance() {
	ticker := time.NewTicker(15 * time.Minute)
	for {
		usersRefreshed, refreshError := spotify.RefreshExpiringTokens(globalClient)
		log.Println("Spotify token refresh function refreshed", usersRefreshed, "tokens.")
		if refreshError != nil {
			log.Println("Spotify token refresh function exited early due to error:", refreshError)
		}
		<-ticker.C // Block until ticker kicks a tick off
	}
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

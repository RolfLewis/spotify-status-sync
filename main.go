package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/routes"
	"rolflewis.com/spotify-status-sync/src/slack"
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

	router.GET("/spotify/callback", spotifyCallbackClientInjector)
	router.GET("/slack/callback", slackCallbackClientInjector)

	router.POST("/slack/events", eventsClientInjector)
	router.POST("/slack/interactivity", interactionsClientInjector)

	// Database setup
	database.ConnectToDatabase()
	database.ValidateSchema()

	// Kick off the spotify token maintenance routine
	go spotifyTokenMaintenance()
	// Kick of the the currently playing query loop
	go spotifyCurrentlyPlayingLoop()

	// Stand up server
	router.Run(":" + port)
}

func statusSyncHelper() (int, error) {
	// Get all users who have spotify connected
	users, usersError := database.GetAllConnectedUsers()
	if usersError != nil {
		return 0, usersError
	}
	// Get currently playing for each user
	for index, user := range users {
		current, currentError := spotify.GetCurrentlyPlayingForUser(user, globalClient)
		if currentError != nil {
			return index, currentError
		}
		updateError := slack.UpdateUserStatus(user, current.Item.Name, globalClient)
		if updateError != nil {
			return index, updateError
		}
	}
	// return success
	return len(users), nil
}

func spotifyCurrentlyPlayingLoop() {
	ticker := time.NewTicker(5 * time.Second)
	for {
		usersUpdated, updateError := statusSyncHelper()
		log.Println("Spotify Currently Playing synced for", usersUpdated, "users.")
		if updateError != nil {
			log.Println("Spotify Currently Playing Sync exited early due to error:", updateError)
		}
		<-ticker.C // Block until ticker kicks a tick off
	}
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

func slackCallbackClientInjector(context *gin.Context) {
	routes.SlackCallbackFlow(context, globalClient)
}

func spotifyCallbackClientInjector(context *gin.Context) {
	routes.SpotifyCallbackFlow(context, globalClient)
}

func eventsClientInjector(context *gin.Context) {
	routes.EventsEndpoint(context, globalClient)
}

func interactionsClientInjector(context *gin.Context) {
	routes.InteractivityEndpoint(context, globalClient)
}

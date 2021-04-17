package main

import (
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/heroku/x/hmetrics/onload"
)

func main() {
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

	router.GET("/login", loginFlow)
	router.GET("/callback", callbackFlow)

	router.Run(":" + port)
}

func loginFlow(context *gin.Context) {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	callbackURL := "https://spotify-status-sync.herokuapp.com/callback"
	spotifyAuthURL := "https://accounts.spotify.com/authorize?client_id=" + clientID + "&response_type=code&redirect_uri=" + url.PathEscape(callbackURL) + "&scope=user-read-currently-playing"
	context.Redirect(http.StatusOK, spotifyAuthURL)
}

func callbackFlow(context *gin.Context) {
	code := context.Query("code")
	errorMsg := context.Query("error")

	if errorMsg != "" {
		context.String(http.StatusInternalServerError, errorMsg)
	} else {
		context.String(http.StatusOK, code)
	}
}

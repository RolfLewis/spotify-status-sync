package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	_ "github.com/heroku/x/hmetrics/onload"
)

var appURL = "https://spotify-status-sync.herokuapp.com/"
var spotifyBaseURL = "https://accounts.spotify.com/"
var spotifyClient *http.Client

type spotifyAuthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

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

	// Create the global spotify client
	spotifyClient = http.DefaultClient

	router.Run(":" + port)
}

func getLoginRedirectURL() string {
	return appURL + "callback"
}

func loginFlow(context *gin.Context) {
	callbackURL := getLoginRedirectURL()

	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
	queryValues.Set("response_type", "code")
	queryValues.Set("redirect_uri", url.PathEscape(callbackURL))
	queryValues.Set("scope", "user-read-currently-playing")

	// Redirect to spotify OAuth page
	spotifyAuthURL := spotifyBaseURL + "/authorize?" + queryValues.Encode()
	context.Redirect(http.StatusPermanentRedirect, spotifyAuthURL)
}

func callbackFlow(context *gin.Context) {
	code := context.Query("code")
	errorMsg := context.Query("error")

	if errorMsg != "" {
		context.String(http.StatusInternalServerError, errorMsg)
		return
	}

	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("grant_type", "authorization_code")
	queryValues.Set("code", code)
	queryValues.Set("redirect_uri", getLoginRedirectURL())
	urlEncodedBody := queryValues.Encode()

	// Get the auth and refresh tokens
	req, reqError := http.NewRequest(http.MethodPost, spotifyBaseURL+"api/token", strings.NewReader(urlEncodedBody))
	if reqError != nil {
		context.String(http.StatusInternalServerError, reqError.Error())
		return
	}

	// Add the body headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(urlEncodedBody)))

	// Encode the authorization header
	bytes := []byte(os.Getenv("SPOTIFY_CLIENT_ID") + ":" + os.Getenv("SPOTIFY_CLIENT_SECRET"))
	req.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString(bytes))

	// Send the request
	resp, respError := spotifyClient.Do(req)
	if respError != nil {
		context.String(http.StatusInternalServerError, respError.Error())
		return
	}
	defer resp.Body.Close()

	// Check status codes
	if resp.StatusCode != http.StatusOK {
		context.String(http.StatusInternalServerError, "Non-200 status code from auth endpoint: "+strconv.Itoa(resp.StatusCode)+" / "+resp.Status)
		return
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(resp.Body)
	if readError != nil {
		context.String(http.StatusInternalServerError, readError.Error())
		return
	}

	log.Println(jsonBytes)

	var tokens spotifyAuthResponse
	jsonError := json.Unmarshal(jsonBytes, &tokens)
	if jsonError != nil {
		context.String(http.StatusInternalServerError, jsonError.Error())
		return
	}

	context.String(http.StatusOK, tokens.AccessToken)
}

func writeSpotifyToken(token string) {
	tokenFile, fileError := os.Create("spotify-token")
	if fileError != nil {
		log.Fatal(fileError)
	}
	written, writeError := tokenFile.WriteString(token)
	if writeError != nil {
		log.Fatal(writeError)
	}
	if written != len(token) {
		log.Fatal(errors.New("Full token not written"))
	}
}

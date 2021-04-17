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
	"github.com/google/uuid"
	_ "github.com/heroku/x/hmetrics/onload"
)

var appURL = "https://spotify-status-sync.herokuapp.com/"
var spotifyAuthURL = "https://accounts.spotify.com/"
var spotifyAPIURL = "https://api.spotify.com/v1/"
var spotifyClient *http.Client

type spotifyAuthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type spotifyProfile struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
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

	router.GET("/spotify/login", loginFlow)
	router.GET("/spotify/callback", callbackFlow)
	router.POST("/interactivity", interactivityEndpoint)

	// Create the global spotify client
	spotifyClient = http.DefaultClient

	// Database setup
	connectToDatabase()
	validateSchema()

	router.Run(":" + port)
}

func interactivityEndpoint(context *gin.Context) {
	var jsonData string
	context.BindJSON(&jsonData)
	log.Println(jsonData)
}

func getLoginRedirectURL() string {
	return appURL + "callback"
}

func loginFlow(context *gin.Context) {
	callbackURL := getLoginRedirectURL()

	// Check for cookies
	sessionCookie, cookieError := context.Cookie("session")
	if cookieError != nil {
		sessionCookie = uuid.NewString()
		context.SetCookie("session", sessionCookie, 3600, "/", "spotify-status-sync.herokuapp.com", true, true)
	}

	// Check if spotify has been connected yet for this session
	profileID, tokens, dbError := getSpotifyForSession(sessionCookie)
	if dbError != nil {
		context.String(http.StatusInternalServerError, dbError.Error())
		return
	}

	if profileID == "" { // Spotify has not been connected in this session yet
		// Set the query values
		queryValues := url.Values{}
		queryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
		queryValues.Set("response_type", "code")
		queryValues.Set("redirect_uri", url.PathEscape(callbackURL))
		queryValues.Set("scope", "user-read-currently-playing")

		// Redirect to spotify OAuth page
		OAuthURL := spotifyAuthURL + "/authorize?" + queryValues.Encode()
		context.Redirect(http.StatusPermanentRedirect, OAuthURL)
	} else { // Spotify already exists for this session
		context.String(http.StatusOK, "Spotify already connected for this session: "+profileID+" / "+tokens.AccessToken)
	}
}

func callbackFlow(context *gin.Context) {
	// Get the session ID cookie
	session, sessionError := context.Cookie("session")
	if sessionError != nil {
		context.String(http.StatusInternalServerError, sessionError.Error())
		return
	}

	// Check for error from Spotify
	errorMsg := context.Query("error")
	if errorMsg != "" {
		context.String(http.StatusInternalServerError, errorMsg)
		return
	}

	// Read auth code
	code := context.Query("code")

	// Exchange code for tokens
	tokens, exchangeError := exchangeCodeForTokens(code)
	if exchangeError != nil {
		context.String(http.StatusInternalServerError, exchangeError.Error())
		return
	}

	// Get the user's profile information
	profile, profileError := getProfileForTokens(*tokens)
	if profileError != nil {
		context.String(http.StatusInternalServerError, profileError.Error())
		return
	}

	if profile == nil {
		context.String(http.StatusInternalServerError, "No profile returned from GET")
		return
	}

	// Save the information to the DB
	dbError := addSpotifyToSession(session, *profile, *tokens)
	if dbError != nil {
		context.String(http.StatusInternalServerError, dbError.Error())
		return
	}

	context.String(http.StatusOK, session+" "+profile.DisplayName+" "+profile.ID)
}

func exchangeCodeForTokens(code string) (*spotifyAuthResponse, error) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("grant_type", "authorization_code")
	queryValues.Set("code", code)
	queryValues.Set("redirect_uri", getLoginRedirectURL())
	urlEncodedBody := queryValues.Encode()

	// Get the auth and refresh tokens
	authReq, authReqError := http.NewRequest(http.MethodPost, spotifyAuthURL+"api/token", strings.NewReader(urlEncodedBody))
	if authReqError != nil {
		return nil, authReqError
	}

	// Add the body headers
	authReq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	authReq.Header.Add("Content-Length", strconv.Itoa(len(urlEncodedBody)))

	// Encode the authorization header
	bytes := []byte(os.Getenv("SPOTIFY_CLIENT_ID") + ":" + os.Getenv("SPOTIFY_CLIENT_SECRET"))
	authReq.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString(bytes))

	// Send the request
	authResp, authRespError := spotifyClient.Do(authReq)
	if authRespError != nil {
		return nil, authRespError
	}
	defer authResp.Body.Close()

	// Check status codes
	if authResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from auth endpoint: " + strconv.Itoa(authResp.StatusCode) + " / " + authResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(authResp.Body)
	if readError != nil {
		return nil, readError
	}

	var tokens spotifyAuthResponse
	jsonError := json.Unmarshal(jsonBytes, &tokens)
	if jsonError != nil {
		return nil, jsonError
	}

	return &tokens, nil
}

func getProfileForTokens(tokens spotifyAuthResponse) (*spotifyProfile, error) {
	// Build request
	profReq, profReqError := http.NewRequest(http.MethodGet, spotifyAPIURL+"me", nil)
	if profReqError != nil {
		return nil, profReqError
	}

	// Add auth
	profReq.Header.Add("Authorization", "Bearer "+tokens.AccessToken)

	// Send the request
	profResp, profRespError := spotifyClient.Do(profReq)
	if profRespError != nil {
		return nil, profRespError
	}
	defer profResp.Body.Close()

	// Check status codes
	if profResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from profile endpoint: " + strconv.Itoa(profResp.StatusCode) + " / " + profResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(profResp.Body)
	if readError != nil {
		return nil, readError
	}

	var profile spotifyProfile
	jsonError := json.Unmarshal(jsonBytes, &profile)
	if jsonError != nil {
		return nil, jsonError
	}

	return &profile, nil
}

package util

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Takes an error and handles logging it and reporting a 500. Returns true if error was non-nil
func InternalError(err error, context *gin.Context) bool {
	if err != nil {
		log.Println(err.Error())
		context.String(http.StatusInternalServerError, err.Error())
		return true
	}
	return false
}

// Ensures that a payload / message is directly from Slack and enforces several of their security methods
func IsSecureFromSlack(context *gin.Context) bool {
	version := "v0" // This is a slack constant currently
	timestampString := context.GetHeader("X-Slack-Request-Timestamp")

	if timestampString == "" {
		return false
	}

	timestamp, tsError := strconv.ParseInt(timestampString, 10, 64)
	if tsError != nil {
		return false
	}

	// Verify that this timestamp is in the last 2 minutes - mitigates replay attacks
	if math.Abs(time.Since(time.Unix(timestamp, 0)).Seconds()) > 2*60 {
		return false
	}

	// Copy the body buffer out, read it, and replace it
	var bodyBytes []byte
	if context.Request.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(context.Request.Body)
	}
	context.Request.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

	// Compute signature and compare
	totalString := version + ":" + strconv.FormatInt(timestamp, 10) + ":" + string(bodyBytes)
	hasher := hmac.New(sha256.New, []byte(os.Getenv("SLACK_SIGNING_KEY")))
	hasher.Write([]byte(totalString))
	mySignature := "v0=" + hex.EncodeToString(hasher.Sum(nil))
	providedSignature := context.GetHeader("X-Slack-Signature")

	// If the signature header was not provided, not sent by slack
	if providedSignature == "" {
		return false
	}

	// If the calculated and given sigs don't match, not sent by slack
	if !hmac.Equal([]byte(mySignature), []byte(providedSignature)) {
		return false
	}

	return true
}

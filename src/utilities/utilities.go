package utilities

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// LogEvent -- writes execution attempt to file and alerts user if plugin execution aborted due to log error
func LogEvent(pluginExecutionLog, plugin, loader string, queryParameters url.Values, w http.ResponseWriter) error {

	logfile, err := os.OpenFile(pluginExecutionLog, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	var logWriteError error
	userNotificationErrorMsg := "Error opening or writing plugin execution to log.  Aborting.  Please contact Devops"

	if err != nil {
		logWriteError = fmt.Errorf("Error opening %s for append.  Reason: %s.",
			pluginExecutionLog,
			err)

		fmt.Fprint(w, userNotificationErrorMsg)
		return logWriteError
	}

	defer logfile.Close()

	_, err = logfile.WriteString(
		fmt.Sprintf(
			"Time: [%v] :: User: [ %s ] attempted to use loader: [ %s ] to execute plugin: [ %s ] with args: [ %s ]\n",
			time.Now().Format(time.RFC850), // http://stackoverflow.com/questions/5885486/how-do-i-get-the-current-timestamp-in-go
			queryParameters.Get("user_name"),
			loader,
			plugin,
			queryParameters.Get("text")))

	if err != nil {
		logWriteError = fmt.Errorf("** Error writing to %s.  Reason: %s",
			pluginExecutionLog,
			err)

		fmt.Fprint(w, userNotificationErrorMsg)
		return logWriteError
	}

	return nil
}

// SendErrorToSlack -- in case the plugin fails to execute, we'll send back any exceptions/stdout from plugin
func SendErrorToSlack(responseURL string, output string) {
	// escape quotes in output, so it doesn't break our json payload
	output = strings.Replace(output, "\"", "\\\"", -1)

	var errorPayload = []byte("{\"text\": \"exception or error occurred in plugin:\n```" + output + "```\"}")
	req, err := http.NewRequest("POST", responseURL, bytes.NewBuffer(errorPayload))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		fmt.Printf("Could not send error message to slack via HTTP POST.  Error: %v", err)
		return
	}
	resp.Body.Close()
}

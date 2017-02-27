package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"utilities"

	"gopkg.in/alexcesaro/statsd.v2"
)

// Due to Go having different command line argument syntax than Python, this is probably necessary
func prepareCommandLineArgSyntax(loader string, queryParameters url.Values) []string {
	var cmdLineArgList []string

	// Loop through each query parameter passed in via Slack/Jenkins and use the appropriate syntax for the loader specified
	// Go cmd arg parse library likes format "--arg=value" whereas Python's argparse likes "-arg value"
	for key, value := range queryParameters {
		// Skip keys that won't be used in the plugins
		if key == "plugin" || key == "loader" {
			continue
		}
		if loader == "python" {
			cmdLineArgList = append(cmdLineArgList, []string{fmt.Sprintf("-%s", key), fmt.Sprintf("\"%s\"", value[0])}...)
		} else if loader == "go" {
			cmdLineArgList = append(cmdLineArgList, []string{fmt.Sprintf("--%s=\"%s\"", key, value[0])}...)
		}
	}
	return cmdLineArgList
}

// Wrap our "/" handler in a closure, so we only need to connect to statsd host one time
func initializeHandler(client *statsd.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		loaderOpts := map[string]string{
			"python":  "python",
			"python3": "python3",
			"node":    "node",
			"go":      "go run",
		}

		queryParameters := r.URL.Query()
		plugin := queryParameters.Get("plugin")
		loader := queryParameters.Get("loader")

		// We'll use this to drop the file extension of the plugin from the statsd records
		statsdPluginRegex := regexp.MustCompile("\\.\\w+")

		// Due to "go run" being two words, I have to break up the command into components
		// And then concatenate the commandline args
		var loaderShellCommand string
		var shellCommandParts []string
		var remainderOfShellCommand []string

		// This is used by automated jobs like Jenkins Build/Deploys
		jobRunnerURL := queryParameters.Get("jobRunnerUrl")

		// Our execution log is under /opt/slackcmdhandler/ on dvblzut0
		pluginExecutionLog := "plugin-execution.log"

		// We want to ensure that the loader specified is valid
		loaderAcceptable := false

		// Get the actual shell command we should run for the loader in question
		for loaderKey, binary := range loaderOpts {
			if loaderKey == loader {
				loaderAcceptable = true
				loaderShellCommand = binary
			}
		}

		// If plugin wasn't passed in as part of the query, abort.
		if plugin == "" || !loaderAcceptable {
			http.Error(w, "Plugin request not in appropriate format.\n", http.StatusBadRequest)
			return
		}

		// break our loader binary into multiple fields if applicable
		// this is in case our binary is multiple words (e.g., "go run")
		shellCommandParts = strings.Fields(loaderShellCommand)
		firstPartOfShellCommand := shellCommandParts[0]
		if len(loaderShellCommand) > 1 {
			remainderOfShellCommand = shellCommandParts[1:len(shellCommandParts)]
		} else {
			remainderOfShellCommand = []string{}
		}

		// Each language has a preferred way of handling commandline arg syntax
		slackArgList := prepareCommandLineArgSyntax(loader, queryParameters)

		// Append the actual plugin we'll be executing
		remainderOfShellCommand = append(remainderOfShellCommand,
			fmt.Sprintf("/opt/slack-plugins/%s/%s", loader, plugin))

		// Append command line arguments that will be passed to plugin
		remainderOfShellCommand = append(remainderOfShellCommand, slackArgList...)

		// Send an immediate response to user to let them know the plugin did not return an error on run
		fmt.Fprintf(w, "Executing plugin: %s. It should send additional output shortly...\n", plugin)

		// Logging plugin execution request
		if err := utilities.LogEvent(pluginExecutionLog, plugin, loader, queryParameters, w); err != nil {
			log.Printf("Error logging event. %s\nAborted execution.\n", err)
			return
		}

		// Inc usage of partcular plugin inside Statsd Host.
		// Using regex to remove file extension, so statsd isn't confused
		client.Increment(statsdPluginRegex.ReplaceAllString(plugin, ""))

		// If the query parameter is set for jobRunnerUrl then...
		// let's wait until the request returns (instead of spawning a go routine) then send the salt response back to the caller
		if jobRunnerURL != "" {

			output, err := exec.Command(
				firstPartOfShellCommand,
				remainderOfShellCommand...).CombinedOutput()

			if err != nil {
				errorMessage := fmt.Sprintf("Failure executing %s.", plugin)
				http.Error(w, errorMessage, http.StatusInternalServerError)
				return
			}

			// Send stdout/stderr from plugin back to caller (e.g., Jenkins)
			fmt.Fprintf(w, "%s", output)

		} else {
			// Spin up a go routine and let it run after this function returns
			// If i didn't do this, Slack wouldn't receive our 'Executing plugin...' message until after the plugin finished.
			go func(queryParameters url.Values, loader, plugin string) {

				output, err := exec.Command(
					firstPartOfShellCommand,
					remainderOfShellCommand...,
				).CombinedOutput()

				if err != nil {
					errorMessage := fmt.Sprintf("Failure executing %s.", plugin)
					http.Error(w, errorMessage, http.StatusInternalServerError)

					// Something borked (exception, bad exit code, etc).  Let's notify user that attempted to execute plugin
					utilities.SendErrorToSlack(queryParameters.Get("response_url"), string(output))
					log.Printf("\n*** Failure executing %s.  Reason: %s\nOutput: %s", plugin, err, output)
				} else {
					// Rely on plugin to send output in its desired format.
					// We'll just output overall success to stdout
					successMessage :=
						fmt.Sprintf("\nPlugin %s ran with args: %s -- successfully",
							plugin,
							queryParameters.Get("text"))

					log.Println(successMessage)
				}
			}(queryParameters, loader, plugin)
		}
	}
}

func main() {

	client, err := statsd.New(statsd.Address("stats-d-url"), statsd.Prefix("slack-plugin-api."))
	if err != nil {
		log.Fatal(err)
	}

	defer client.Close()

	http.HandleFunc("/", initializeHandler(client))
	err = http.ListenAndServeTLS(":4443", "/ssl/prod.pm.pem", "/ssl/prod.pm.key", nil)

	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

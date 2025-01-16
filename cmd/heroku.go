package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	heroku "github.com/heroku/heroku-go/v5"

	"github.com/spf13/cobra"
)

// herokuCmd run heroku server
var herokuCmd = &cobra.Command{
	Use:   "heroku",
	Short: "Heroku broker",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("invalid command")
		}
		return runHeroku()
	},
}

var client *heroku.Service
var teamName string
var discover bool

func init() {
	herokuCmd.Flags().StringVar(&teamName, "team", "", "limit apps to this team")
	herokuCmd.Flags().BoolVar(&discover, "discover", false, "discover issuer and sub")
	cmd.AddCommand(herokuCmd)
}

func getResourceHeroku(name string, url string, discover bool, updater configUpdater) *Resource {
	iss := url  // the local factor identity provider uses the app url
	sub := name // the local factor identity provider uses the app name
	if discover {
		// TODO: use local run:insade to call `factor issuer` and parse output
	}
	return getResource(name, url, iss, sub, updater)
}

func runHeroku() error {
	token, ok := os.LookupEnv("HEROKU_API_KEY")
	if !ok {
		log.Fatal("HEROKU_API_KEY not set")
	}
	httpClient := &http.Client{
		Transport: &heroku.Transport{
			BearerToken: token,
		},
	}
	client = heroku.NewService(httpClient)

	if t, ok := os.LookupEnv("HEROKU_TEAM"); ok && teamName == "" {
		teamName = t
	}
	if t, ok := os.LookupEnv("DISCOVER"); ok && !discover {
		t = strings.ToUpper(t)
		if t == "1" || t == "T" || t == "TRUE" || t == "Y" || t == "YES" {
			discover = true
		}
	}
	resources := map[string]*Resource{}

	updater := func(name string, vars map[string]*string) error {
		_, err := client.ConfigVarUpdate(context.TODO(), name, vars)
		return err
	}

	if teamName == "" {
		apps, err := client.AppList(context.TODO(), nil)
		if err != nil {
			log.Fatal(err)
		}
		for _, app := range apps {
			r := getResourceHeroku(app.Name, app.WebURL, discover, updater)
			resources[r.ID] = r
		}
	} else {
		apps, err := client.TeamAppListByTeam(context.TODO(), teamName, nil)
		if err != nil {
			log.Fatal(err)
		}
		for _, app := range apps {
			r := getResourceHeroku(app.Name, app.WebURL, discover, updater)
			resources[r.ID] = r
		}
	}

	// TODO: watch for changes to app urls and update the other end of the pipe

	return runBrokerServer("8002", resources)
}

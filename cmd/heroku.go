package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	token, ok := os.LookupEnv("HEROKU_API_KEY")
	if !ok {
		log.Fatal("HEROKU_API_KEY not set")
	}
	herokuCmd.Flags().StringVar(&teamName, "team", "", "limit apps to this team")
	herokuCmd.Flags().BoolVar(&discover, "discover", false, "discover issuer and sub")
	cmd.AddCommand(herokuCmd)
	httpClient := &http.Client{
		Transport: &heroku.Transport{
			BearerToken: token,
		},
	}
	client = heroku.NewService(httpClient)
}

func toConfig(data json.RawMessage, prefix string, config map[string]*string) error {
	if isJSONEmpty(data) {
		return nil
	}
	rawMap := map[string]interface{}{}
	err := json.Unmarshal(data, &rawMap)
	if err != nil {
		return fmt.Errorf("error unmarshaling JSON: %w", err)
	}

	for key, value := range rawMap {
		strValue := fmt.Sprintf("%v", value)
		config[strings.ToUpper(fmt.Sprintf("%s%s", prefix, key))] = &strValue
	}
	return nil
}

type IdentityValidator struct {
	Iss string `json:"iss,omitempty"`
	Sub string `json:"sub,omitempty"`
	Aud string `json:"aud,omitempty"`
}

type IncomingIdentity map[string]IdentityValidator

func toIncomingIdentity(pipe *Pipe, config map[string]*string) error {
	if isJSONEmpty(pipe.Other.Data) {
		return nil
	}
	rawMap := map[string]interface{}{}
	err := json.Unmarshal(pipe.Other.Data, &rawMap)
	if err != nil {
		return fmt.Errorf("error unmarshaling JSON: %w", err)
	}
	// TODO explicitly check that we are the provider of an OIDCAuth adapter
	if _, ok := rawMap["ISS"]; ok {
		iss, ok := rawMap["ISS"].(string)
		if !ok {
			return fmt.Errorf("field 'ISS' is missing or not a string")
		}
		sub, ok := rawMap["SUB"].(string)
		if !ok {
			return fmt.Errorf("field 'sub' is missing or not a string")
		}
		aud, ok := rawMap["AUD"].(string)
		if !ok {
			return fmt.Errorf("field 'aud' is missing or not a string")
		}
		incoming := IncomingIdentity{}
		incoming[pipe.ID] = IdentityValidator{
			Iss: iss,
			Sub: sub,
			Aud: aud,
		}
		incomingJson, err := json.Marshal(&incoming)
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
		incomingStr := string(incomingJson)
		config["INCOMING_IDENTITY"] = &incomingStr
	}
	// TODO explicitly check that we are the consumer of an OIDCAuth adapter
	if _, ok := rawMap["URI"]; ok {
		// set a config var to create an identity token
		thisMap := map[string]interface{}{}
		err := json.Unmarshal(pipe.This.Data, &thisMap)
		if err != nil {
			return fmt.Errorf("error unmarshaling JSON: %w", err)
		}
		if aud, ok := thisMap["AUD"].(string); ok {
			config[strings.ToUpper(fmt.Sprintf("%s_AUDIENCE", pipe.ID))] = &aud
		}
	}
	return nil
}

func updateConfig(name string, pipe *Pipe) error {
	vars := map[string]*string{}
	if err := toConfig(pipe.This.Data, fmt.Sprintf("PIPE_%s_THIS_", pipe.ID), vars); err != nil {
		return err
	}
	if err := toConfig(pipe.Other.Data, fmt.Sprintf("PIPE_%s_OTHER_", pipe.ID), vars); err != nil {
		return err
	}
	// TODO support multiple connections by aggregating all of the pipes
	if err := toIncomingIdentity(pipe, vars); err != nil {
		return err
	}

	// if the pipe has an oidc adapter set the config on this end
	log.Infof("Updating config for %s with %+v", name, vars)
	client.ConfigVarUpdate(context.TODO(), name, vars)
	return nil
}

func getResource(name string, url string, discover bool) *Resource {
	log.Infof("Adding resource %s at %s", name, url)
	iss := url  // the local factor identity provider uses the app url
	sub := name // the local factor identity provider uses the app name
	if discover {
		// TODO: use heroku run:insade to call `factor issuer` and parse output
	}
	callback := func(pipe *Pipe) error {
		return updateConfig(name, pipe)
	}
	uri := URIData{
		URI: url,
	}
	return &Resource{
		ID:             name,
		DefaultData:    uri,
		UpdateCallback: &callback,
		Pipes:          map[string]*Pipe{},
		Offers: []*Blueprint{
			// register an https+oidc offer to be a backing_service for another app
			NewOffer(
				"backing_service",
				[]AdapterType{OIDCAuth},
				[]*PipeTemplate{
					NewTemplate(
						true,
						OIDCAuth,
						nil,
					),
				},
				[]*PipeTemplate{
					NewTemplate(
						true,
						ProtoHttps,
						URIHttpsData{
							URI: url,
						},
					),
				},
			),
		},
		Needs: []*Blueprint{
			// register an https+oidc need for a backing_service
			NewNeed(
				"backing_service",
				[]AdapterType{OIDCAuth},
				[]*PipeTemplate{
					NewTemplate(
						false,
						OIDCAuth,
						OIDCAuthData{
							Issuer:   iss,
							Subject:  sub,
							Audience: "backing_service", // audience matches the need
						},
					),
				},
				[]*PipeTemplate{
					NewTemplate(
						false,
						ProtoHttps,
						nil,
					),
				},
			),
		},
	}
}

func runHeroku() error {

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

	if teamName == "" {
		apps, err := client.AppList(context.TODO(), nil)
		if err != nil {
			log.Fatal(err)
		}
		for _, app := range apps {
			resources[app.Name] = getResource(app.Name, app.WebURL, discover)
		}
	} else {
		apps, err := client.TeamAppListByTeam(context.TODO(), teamName, nil)
		if err != nil {
			log.Fatal(err)
		}
		for _, app := range apps {
			resources[app.Name] = getResource(app.Name, app.WebURL, discover)
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINFO)

	go func() {
		_, prefix := getPortAndPrefix("8002")
		for {
			<-sigChan
			if r, ok := resources["backend"]; ok {
				r.Mutex.Lock()
				if p, ok := r.Pipes["frontend"]; ok && p.Other.URI != "" {
					token, err := generateToken(prefix, p.Other.URI, p.This.URI)
					if err != nil {
						log.Errorf("Error generating token: %s", err)
					}
					if err := p.This.SetData(URIData{URI: "https://updated.herokuapp.com"}); err != nil {
						log.Errorf("Error updating URI: %s", err)
					}
					updateOther(token, p.Other.URI, p.This.Data)
				}
				r.Mutex.Unlock()
			}
		}
	}()
	return runBrokerServer("8002", resources)
}

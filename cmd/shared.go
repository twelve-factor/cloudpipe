package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)


func parseMetadata(output []byte) (string, string, string, string) {
	var name, url, iss, sub string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			name = value
		case "url":
			url = value 
		case "iss":
			iss = value
		case "sub":
			sub = value
		}
	}
	return name, url, iss, sub
}

type configUpdater func(name string, vars map[string]*string) error

func getResource(name, url, iss, sub string, updater configUpdater) *Resource {
	log.Infof("Adding resource %s at %s", name, url)
	callback := func(pipe *Pipe) error {
		return updateConfig(name, pipe, updater)
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
	if _, ok := rawMap["URI"]; ok {
		thisMap := map[string]interface{}{}
		err := json.Unmarshal(pipe.This.Data, &thisMap)
		if err != nil {
			return fmt.Errorf("error unmarshaling JSON: %w", err)
		}
		if aud, ok := thisMap["AUD"].(string); ok {
			// set a config var to create an identity token
			config[strings.ToUpper(fmt.Sprintf("%s_AUDIENCE", pipe.ID))] = &aud
		}
	}
	return nil
}

func updateConfig(name string, pipe *Pipe, updater configUpdater) error {
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

	log.Infof("Updating config for %s with %+v", name, vars)
	return updater(name, vars)
}
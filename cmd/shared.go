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

// configRetriever returns environment variables
type configRetriever func() []string

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

type ClientCredsData struct {
	Iss string `json:"iss"`
	Sub string `json:"sub"`
	Aud string `json:"aud"`
}

type ClientCreds struct {
	Type     string         `json:"type"`
	ClientID string         `json:"client_id,omitempty"`
	Data     ClientCredsData `json:"data"`
}

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
		
		// Create the client credentials structure
		clientCreds := ClientCreds{
			Type: "oidc",
			Data: ClientCredsData{
				Iss: iss,
				Sub: sub,
				Aud: aud,
			},
		}
		
		clientCredsJson, err := json.Marshal(&clientCreds)
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
		clientCredsStr := string(clientCredsJson)
		config[fmt.Sprintf("%s_CLIENT_CREDS", strings.ToUpper(pipe.ID))] = &clientCredsStr
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
	
	// Save the original pipe ID to preserve case
	origID := pipe.ID
	vars[fmt.Sprintf("PIPE_%s_ID", pipe.ID)] = &origID
	
	log.Infof("Updating config for %s with %+v", name, vars)
	return updater(name, vars)
}

// buildPipesFromEnv scans environment variables to recreate pipes
func buildPipesFromEnv(resource *Resource, retriever configRetriever) {
	// Collect pipe IDs from environment variables
	pipeIDs := make(map[string]bool)
	origIDMap := make(map[string]string) // Map from uppercase ID to original case ID
	
	// Get environment variables for this resource
	envVars := retriever()
	
	// First pass: find original IDs
	for _, env := range envVars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := parts[0]
		value := parts[1]
		
		// Check for saved original IDs
		if strings.HasSuffix(key, "_ID") && strings.HasPrefix(key, "PIPE_") {
			pipeIDUpper := strings.TrimSuffix(strings.TrimPrefix(key, "PIPE_"), "_ID")
			pipeIDs[pipeIDUpper] = true
			origIDMap[pipeIDUpper] = value
			log.Debugf("Found original pipe ID: %s -> %s", pipeIDUpper, value)
		}
	}
	
	// Create pipes for each identified ID
	log.Infof("Found %d pipe IDs: %v", len(pipeIDs), pipeIDs)
	for pipeID := range pipeIDs {
		// Use original ID if available
		origID := pipeID
		if savedID, ok := origIDMap[strings.ToUpper(pipeID)]; ok {
			origID = savedID
			log.Debugf("Using original case for pipe ID: %s", origID)
		}
		
		pipe := reconstructPipe(pipeID, origID, retriever)
		if pipe != nil {
			resource.Pipes[origID] = pipe
			
			// Extract issuer/URI info for better logging
			var thisInfo, otherInfo string
			
			var thisData, otherData map[string]interface{}
			if !isJSONEmpty(pipe.This.Data) {
				if err := json.Unmarshal(pipe.This.Data, &thisData); err == nil {
					if uri, ok := thisData["URI"].(string); ok {
						thisInfo = uri
					}
				}
			}
			
			if !isJSONEmpty(pipe.Other.Data) {
				if err := json.Unmarshal(pipe.Other.Data, &otherData); err == nil {
					if iss, ok := otherData["ISS"].(string); ok {
						otherInfo = iss
					}
				}
			}
			
			if thisInfo != "" && otherInfo != "" {
				log.Infof("Loaded pipe %s from environment variables: %s ↔ %s", origID, thisInfo, otherInfo)
			} else {
				log.Infof("Loaded pipe %s from environment variables", origID)
			}
		} else {
			log.Warnf("Failed to reconstruct pipe %s from environment variables", origID)
		}
	}
}

// reconstructPipe builds a pipe object from environment variables
func reconstructPipe(pipeID string, origID string, retriever configRetriever) *Pipe {
	pipe := &Pipe{
		ID:    origID, // Use original case ID
		This:  End{},
		Other: End{},
	}
	
	// Try to reconstruct pipe data from env vars
	thisData := make(map[string]interface{})
	otherData := make(map[string]interface{})
	
	// Get environment variables
	envVars := retriever()
	
	// Look for THIS data
	prefix := fmt.Sprintf("PIPE_%s_THIS_", pipeID)
	for _, env := range envVars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := parts[0]
		value := parts[1]
		
		// Case-insensitive match for pipe prefixes
		upperKey := strings.ToUpper(key)
		upperPrefix := strings.ToUpper(prefix)
		
		if strings.HasPrefix(upperKey, upperPrefix) {
			fieldName := strings.TrimPrefix(upperKey, upperPrefix)
			thisData[fieldName] = value
			log.Debugf("Found THIS data for pipe %s: %s=%s", origID, fieldName, value)
		}
	}
	
	// Look for OTHER data
	prefix = fmt.Sprintf("PIPE_%s_OTHER_", pipeID)
	for _, env := range envVars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := parts[0]
		value := parts[1]
		
		// Case-insensitive match for pipe prefixes
		upperKey := strings.ToUpper(key)
		upperPrefix := strings.ToUpper(prefix)
		
		if strings.HasPrefix(upperKey, upperPrefix) {
			fieldName := strings.TrimPrefix(upperKey, upperPrefix)
			otherData[fieldName] = value
			log.Debugf("Found OTHER data for pipe %s: %s=%s", origID, fieldName, value)
		}
	}
	
	// Convert maps to JSON
	if len(thisData) > 0 {
		thisJSON, err := json.Marshal(thisData)
		if err == nil {
			pipe.This.Data = json.RawMessage(thisJSON)
		} else {
			log.Warnf("Failed to marshal THIS data for pipe %s: %v", origID, err)
		}
	}
	
	if len(otherData) > 0 {
		otherJSON, err := json.Marshal(otherData)
		if err == nil {
			pipe.Other.Data = json.RawMessage(otherJSON)
		} else {
			log.Warnf("Failed to marshal OTHER data for pipe %s: %v", origID, err)
		}
	}
	
	// Only return the pipe if we have some data
	if len(thisData) > 0 || len(otherData) > 0 {
		return pipe
	}
	
	return nil
}
package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
)

// Configuration struct
type ServerConfig struct {
	Prefix string
}

type contextKey string

const configKey contextKey = "config"

// Middleware to add configuration to the context
func configMiddleware(config ServerConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), configKey, config)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getPortAndPrefix(port string) (string, string) {
	if p, ok := os.LookupEnv("PORT"); ok {
		port = p
	}
	prefix := fmt.Sprintf("http://localhost:%s", port)
	if r, ok := os.LookupEnv("ROOT_URL"); ok {
		prefix = r
	}
	return port, prefix
}

func runBrokerServer(port string, resources map[string]*Resource) error {
	api := http.NewServeMux()
	registerPipeRoutes(api, resources)
	registerOIDCRoutes(api)
	config := ServerConfig{}
	port, config.Prefix = getPortAndPrefix(port)

	log.Infof("Listening on :%s...", port)
	return http.ListenAndServe(fmt.Sprintf(":%s", port), configMiddleware(config, api))
}

func debug(w http.ResponseWriter, req *http.Request) {
	// Buffer to store request details
	var requestDetails strings.Builder

	// Write request details to the buffer
	requestDetails.WriteString(fmt.Sprintf("Method: %s\n", req.Method))
	requestDetails.WriteString(fmt.Sprintf("URL: %s\n", req.URL))
	requestDetails.WriteString(fmt.Sprintf("Proto: %s\n", req.Proto))
	for name, headers := range req.Header {
		for _, h := range headers {
			requestDetails.WriteString(fmt.Sprintf("Header: %s: %s\n", name, h))
		}
	}

	// Log request
	log.Info("Received request:\n", requestDetails.String())

	// Set response header
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/plain")

	// Write request dump into the response
	if _, err := io.WriteString(w, requestDetails.String()); err != nil {
		log.Error("Failed to write response: ", err)
	}

	// Log response
	log.Info("Sent response for debug endpoint")
}

type resourceHandler func(*Resource, http.ResponseWriter, *http.Request)

func unwrapResource(resources map[string]*Resource, rh resourceHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if resource, ok := resources[id]; ok {
			rh(resource, w, r)
			return
		}
		// return 404
		http.Error(w, fmt.Sprintf("Resource '%s' not found", id), http.StatusNotFound)
	}
}

func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(auth, " ", 2)

		if len(parts) != 2 || parts[0] != "Basic" {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		payload, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		credentials := strings.SplitN(string(payload), ":", 2)
		if len(credentials) != 2 {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		username, password := credentials[0], credentials[1]

		// hack to demonstrate we can have some kind of auth
		if username != "foo" || password != "bar" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// If the credentials are valid, proceed to the next handler
		next.ServeHTTP(w, r)
	})
}

func validateAgainstPipe(ctx context.Context, token string, pipe *Pipe) error {
	var val Validator
	var err error
	val.Iss, err = regexp.Compile(pipe.Other.Issuer)
	if err != nil {
		return err
	}
	val.Aud, err = regexp.Compile(pipe.This.URI)
	if err != nil {
		return err
	}
	val.Sub, err = regexp.Compile(pipe.Other.URI)
	if err != nil {
		return err
	}
	if err := Validate(ctx, token, val); err != nil {
		return fmt.Errorf("failed to Validate token: %w", err)
	}
	return nil
}

func oidcAuth(resources map[string]*Resource, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" && strings.HasPrefix(auth, "Bearer ") {
			// Try Bearer Auth
			token := strings.TrimPrefix(auth, "Bearer ")
			id := r.PathValue("id")
			if resource, ok := resources[id]; ok {
				resource.Mutex.RLock()
				pid := r.PathValue("pid")
				if pipe, ok := resource.Pipes[pid]; ok {
					if err := validateAgainstPipe(r.Context(), token, pipe); err != nil {
						log.Errorf("Invalid token: %s", err)
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
					resource.Mutex.RUnlock()
					next.ServeHTTP(w, r)
					return
				}
				resource.Mutex.RUnlock()
			}
		}

		// failed this auth so try basic
		basicAuth(next).ServeHTTP(w, r)
	})
}

func registerPipeRoutes(api *http.ServeMux, resources map[string]*Resource) {
	api.Handle("/debug", http.HandlerFunc(debug))
	api.Handle("/{id}/pipes", basicAuth(unwrapResource(resources, pipesHandler)))
	api.Handle("/{id}/pipes/{pid}", oidcAuth(resources, unwrapResource(resources, pipeHandler)))
	api.Handle("/{id}/needs", basicAuth(unwrapResource(resources, readNeeds)))
	api.Handle("/{id}/offers", basicAuth(unwrapResource(resources, readOffers)))
	api.Handle("/{id}/needs/{sid}", basicAuth(unwrapResource(resources, readNeed)))
	api.Handle("/{id}/offers/{sid}", basicAuth(unwrapResource(resources, readOffer)))
	api.Handle("/{id}/needs/{sid}/adapters", basicAuth(unwrapResource(resources, readNeedAdapters)))
	api.Handle("/{id}/offers/{sid}/adapters", basicAuth(unwrapResource(resources, readOfferAdapters)))
	api.Handle("/{id}/needs/{sid}/protos", basicAuth(unwrapResource(resources, readNeedProtos)))
	api.Handle("/{id}/offers/{sid}/protos", basicAuth(unwrapResource(resources, readOfferProtos)))
	api.Handle("/{id}/needs/{sid}/adapters/{tid}", basicAuth(unwrapResource(resources, readNeedAdapter)))
	api.Handle("/{id}/offers/{sid}/adapters/{tid}", basicAuth(unwrapResource(resources, readOfferAdapter)))
	api.Handle("/{id}/needs/{sid}/protos/{tid}", basicAuth(unwrapResource(resources, readNeedProto)))
	api.Handle("/{id}/offers/{sid}/protos/{tid}", basicAuth(unwrapResource(resources, readOfferProto)))
	api.Handle("/{id}/needs/{sid}/bindings", basicAuth(unwrapResource(resources, needsBindingsHandler)))
	api.Handle("/{id}/offers/{sid}/bindings", basicAuth(unwrapResource(resources, offersBindingsHandler)))
	// TODO: make a redirect at bindings/{name}
}

func needsBindingsHandler(resource *Resource, w http.ResponseWriter, r *http.Request) {
	bindingsHandler(resource, resource.Needs, w, r)
}

func offersBindingsHandler(resource *Resource, w http.ResponseWriter, r *http.Request) {
	bindingsHandler(resource, resource.Offers, w, r)
}

func bindingsHandler(resource *Resource, blueprints []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid := r.PathValue("sid")
	for _, s := range blueprints {
		if sid == s.Name {
			var b Binding
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			adapters := s.DefaultAdapters
			if len(b.Adapters) == 0 {
				b.Adapters = adapters
			}
			missing := []AdapterType{}
			templates := []*PipeTemplate{}
		outer:
			for _, want := range b.Adapters {
				for _, have := range s.Adapters {
					if want == have.ID {
						templates = append(templates, have)
						break outer
					}
				}
				missing = append(missing, want)
			}
			if len(missing) > 0 {
				http.Error(w, fmt.Sprintf("Adapters '%v' not found", missing), http.StatusNotFound)
			}
			proto := s.Protos[0]
			if b.Proto == "" {
				b.Proto = proto.ID.(ProtoType)
			} else {
				found := false
				for _, t := range s.Protos {
					if t.ID == b.Proto {
						proto = t
						found = true
						break
					}
				}
				if !found {
					http.Error(w, fmt.Sprintf("Proto '%s' not found", b.Proto), http.StatusNotFound)
				}
			}
			templates = append(templates, proto)
			sc := r.Context().Value(configKey).(ServerConfig)
			if createPipe(resource, w, &b.Pipe, &sc, s, templates) {
				path := fmt.Sprintf("%s%s", sc.Prefix, strings.TrimSuffix(r.URL.Path, "/bindings"))
				b.Pipe.Links.Blueprint = &Link{Href: path}
				links := []*Link{}
				for _, item := range b.Adapters {
					links = append(links, &Link{Href: fmt.Sprintf("%s/adapters/%s", path, item)})
				}
				b.Pipe.Links.Adapters = links
				b.Pipe.Links.Proto = &Link{Href: fmt.Sprintf("%s/protos/%s", path, b.Proto)}
				location := fmt.Sprintf("/%s/pipes/%s", resource.ID, b.Pipe.ID)
				w.Header().Set("Location", location)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(b)
			}
			return
		}
	}
	http.Error(w, fmt.Sprintf("Blueprint '%s' not found", sid), http.StatusNotFound)
}

func readNeeds(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readBlueprints(resource.Needs, w, r)
}

func readOffers(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readBlueprints(resource.Offers, w, r)
}

func readBlueprints(s []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(s)
}

func readNeed(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readBlueprint(resource.Needs, w, r)
}

func readOffer(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readBlueprint(resource.Offers, w, r)
}

func readBlueprint(blueprints []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid := r.PathValue("sid")
	for _, s := range blueprints {
		if sid == s.Name {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(s)
			return
		}
	}
	http.Error(w, fmt.Sprintf("Blueprint '%s' not found", sid), http.StatusNotFound)
}
func readNeedAdapters(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readAdapters(resource.Needs, w, r)
}

func readOfferAdapters(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readAdapters(resource.Offers, w, r)
}

func readNeedProtos(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readProtos(resource.Needs, w, r)
}

func readOfferProtos(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readProtos(resource.Offers, w, r)
}

func readAdapters(blueprints []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid := r.PathValue("sid")
	for _, s := range blueprints {
		if sid == s.Name {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(s.Adapters)
			return
		}
	}
	http.Error(w, fmt.Sprintf("Blueprint '%s' not found", sid), http.StatusNotFound)
}
func readProtos(blueprints []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid := r.PathValue("sid")
	for _, s := range blueprints {
		if sid == s.Name {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(s.Protos)
			return
		}
	}
	http.Error(w, fmt.Sprintf("Blueprint '%s' not found", sid), http.StatusNotFound)
}

func readNeedAdapter(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readAdapter(resource.Needs, w, r)
}

func readOfferAdapter(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readAdapter(resource.Offers, w, r)
}

func readNeedProto(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readProto(resource.Needs, w, r)
}

func readOfferProto(resource *Resource, w http.ResponseWriter, r *http.Request) {
	readProto(resource.Offers, w, r)
}

func readAdapter(blueprints []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid := r.PathValue("sid")
	for _, s := range blueprints {
		if sid == s.Name {
			tid := r.PathValue("tid")
			for _, t := range s.Adapters {
				if t.ID == AdapterType(tid) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(t)
					return
				}
			}
			http.Error(w, fmt.Sprintf("Auth '%s' not found", tid), http.StatusNotFound)
			return
		}
	}
	http.Error(w, fmt.Sprintf("Blueprint '%s' not found", sid), http.StatusNotFound)
}
func readProto(blueprints []*Blueprint, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid := r.PathValue("sid")
	for _, s := range blueprints {
		if sid == s.Name {
			tid := r.PathValue("tid")
			for _, t := range s.Protos {
				if t.ID == ProtoType(tid) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(t)
					return
				}
			}
			http.Error(w, fmt.Sprintf("Proto '%s' not found", tid), http.StatusNotFound)
			return
		}
	}
	http.Error(w, fmt.Sprintf("Blueprint '%s' not found", sid), http.StatusNotFound)
}

func pipesHandler(resource *Resource, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		resource.Mutex.RLock()
		defer resource.Mutex.RUnlock()
		readPipes(resource, w)
	case http.MethodPost:
		resource.Mutex.Lock()
		defer resource.Mutex.Unlock()
		var p Pipe
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sc := r.Context().Value(configKey).(ServerConfig)
		// include default data if specified
		ts := []*PipeTemplate{}
		if resource.DefaultData != nil {
			ts = append(ts, &PipeTemplate{
				data: &resource.DefaultData,
			})
		}
		if createPipe(resource, w, &p, &sc, nil, ts) {
			location := fmt.Sprintf("/%s/pipes/%s", resource.ID, p.ID)
			w.Header().Set("Location", location)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(p)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func readPipes(resource *Resource, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resource.Pipes)
}

func createPipe(resource *Resource, w http.ResponseWriter, p *Pipe, sc *ServerConfig, s *Blueprint, ts []*PipeTemplate) bool {
	if _, ok := resource.Pipes[p.ID]; ok {
		http.Error(w, fmt.Sprintf("Pipe '%s' already exists", p.ID), http.StatusConflict)
		return false
	}
	// URI and Issuer are set by server
	location := fmt.Sprintf("/%s/pipes/%s", resource.ID, p.ID)
	p.This.URI = fmt.Sprintf("%s%s", sc.Prefix, location)
	p.Links.Self = &Link{Href: p.This.URI}
	p.This.Issuer = sc.Prefix
	// Merge in server provided strategy info
	if s != nil {
		if !s.AddPipe(p.ID) {
			http.Error(w, "Too many pipes for binding", http.StatusConflict)
			return false
		}
		// write d
		this := []*jsonschema.Schema{}
		other := []*jsonschema.Schema{}
		for _, t := range ts {
			if t.This != nil {
				this = append(this, t.This)
			}
			if t.Other != nil {
				other = append(other, t.Other)
			}
		}
		var err error
		if len(this) > 0 {
			if p.This.Schema, err = combineSchemas(this); err != nil {
				log.Error(err)
				s.DeletePipe(p.ID)
				http.Error(w, fmt.Sprintf("Could not combine schemas: %s", err), http.StatusInternalServerError)
				return false
			}
		}
		if len(other) > 0 {
			if p.Other.Schema, err = combineSchemas(other); err != nil {
				log.Error(err)
				s.DeletePipe(p.ID)
				http.Error(w, fmt.Sprintf("Could not combine schemas: %s", err), http.StatusInternalServerError)
				return false
			}
		}
		p.blueprint = s
	}
	// Merge in server provided data
	for _, t := range ts {
		if t.data != nil {
			if err := p.This.SetData(t.data); err != nil {
				log.Error(err)
				s.DeletePipe(p.ID)
				http.Error(w, fmt.Sprintf("Could not set data: %s", err), http.StatusInternalServerError)
				return false
			}
		}
	}
	resource.Pipes[p.ID] = p
	maybeUpdateOther(p, sc)
	if resource.UpdateCallback != nil {
		// TODO: error handling and retries
		if err := (*resource.UpdateCallback)(p); err != nil {
			log.Errorf("Error calling update callback: %v", err)
		}
	}
	return true
}

func pipeHandler(resource *Resource, w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	resource.Mutex.RLock()
	if pipe, ok := resource.Pipes[pid]; ok {
		resource.Mutex.RUnlock()
		switch r.Method {
		case http.MethodGet:
			resource.Mutex.RLock()
			defer resource.Mutex.RUnlock()
			readPipe(pipe, w)
		case http.MethodPatch:
			resource.Mutex.Lock()
			defer resource.Mutex.Unlock()
			updatePipe(resource, pid, w, r)
		case http.MethodDelete:
			resource.Mutex.Lock()
			defer resource.Mutex.Unlock()
			deletePipe(resource.Pipes, pid, w)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	resource.Mutex.RUnlock()
	http.Error(w, fmt.Sprintf("Pipe '%s' not found", pid), http.StatusNotFound)
}

func readPipe(p *Pipe, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(p)
}

const (
	maxRetries = 5
	baseDelay  = 1 * time.Second  // initial delay
	maxDelay   = 32 * time.Second // maximum delay
)

func updateOther(token string, uri string, data json.RawMessage) {
	pipe := Pipe{
		Other: End{
			Data: data,
		},
	}
	jsonData, err := json.Marshal(pipe)
	if err != nil {
		log.Errorf("Error marshalling json: %v", err)
		return
	}

	// Run the update in a separate goroutine
	go func() {
		for i := 0; i < maxRetries; i++ {
			err = doRequest(token, uri, jsonData)
			if err == nil {
				return
			}

			delay := baseDelay * time.Duration(math.Pow(2, float64(i)))
			if delay > maxDelay {
				delay = maxDelay
			}
			log.Infof("Retrying in %v due to error: %v", delay, err)
			time.Sleep(delay)
		}
		log.Warnf("Failed to send update after %d retries", maxRetries)
	}()
}

func doRequest(token, uri string, jsonData []byte) error {
	req, err := http.NewRequest(http.MethodPatch, uri, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating update request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending update request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("invalid response status: %s", resp.Status)
	}
	return nil
}

func maybeUpdateOther(p *Pipe, sc *ServerConfig) {
	if p.Other.URI != "" && !isJSONEmpty(p.This.Data) {
		token, err := generateToken(sc.Prefix, p.Other.URI, p.This.URI)
		if err != nil {
			log.Errorf("Error generating token: %v", err)
			return
		}
		updateOther(token, p.Other.URI, p.This.Data)
	}
}

func updatePipe(resource *Resource, pid string, w http.ResponseWriter, r *http.Request) {
	// local copy of existing pipe
	p := *resource.Pipes[pid]
	this := p.This
	other := p.Other
	var input Pipe
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := p.Merge(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := p.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resource.Pipes[pid] = &p
	if !p.This.Equals(this) {
		sc := r.Context().Value(configKey).(ServerConfig)
		maybeUpdateOther(&p, &sc)
	}
	if resource.UpdateCallback != nil && (!p.This.Equals(this) || !p.Other.Equals(other)) {
		// TODO: error handling and retries
		if err := (*resource.UpdateCallback)(&p); err != nil {
			log.Errorf("Error calling update callback: %v", err)
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

func deletePipe(pipes map[string]*Pipe, pid string, w http.ResponseWriter) {
	// TODO: notify other end of delete
	if p, ok := pipes[pid]; ok {
		if p != nil && p.blueprint != nil {
			p.blueprint.DeletePipe(p.ID)
		}
	}
	delete(pipes, pid)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

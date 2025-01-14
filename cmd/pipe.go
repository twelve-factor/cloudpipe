package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
)

type ProtoType string
type AdapterType string

const (
	OIDCAuth   AdapterType = "auth:oidc"
	MtlsAuth   AdapterType = "auth:mtls"
	BasicAuth  AdapterType = "auth:basic"
	SecretAuth AdapterType = "auth:secret"
	ServerAuth AdapterType = "auth:server"
	//IpsecEnc AdapterType = "enc:ipsec"
	//PrivateLinkConn AdapterType = "conn:privateLink"
)

var AuthPipeTypes = map[AdapterType][2]any{
	OIDCAuth:   {&OIDCAuthData{}, nil},
	MtlsAuth:   {&MtlsAuthData{}, nil},
	BasicAuth:  {&BasicAuthData{}, nil},
	SecretAuth: {&SecretAuthData{}, nil},
	ServerAuth: {nil, &BasicAuthData{}}, // should probably be reversed basic
}

type OIDCAuthData struct {
	Audience string `json:"AUD"`
	Issuer   string `json:"ISS"`
	Subject  string `json:"SUB"`
}

type BasicAuthData struct {
	User string `json:"USER"`
	Pass string `json:"PASS"`
}

type MtlsAuthData struct {
	ClientCert string `json:"CLIENT_CERT"`
	ClientKey  string `json:"CLIENT_KEY"`
	CACert     string `json:"CA_CERT"`
}

type SecretAuthData struct {
	Secret string `json:"SECRET"`
}

type URIData struct {
	URI string `json:"URI"`
}

type URIHttpsData struct {
	// TODO: improve regex
	URI string `json:"URI" jsonschema:"pattern=^https://"`
}

type URIPostgresqlsData struct {
	// TODO: improve regex
	URI string `json:"URI" jsonschema:"pattern=^postgresqls://"`
}

type URIRedissData struct {
	// TODO: improve regex
	URI string `json:"URI" jsonschema:"pattern=^rediss://"`
}

const (
	ProtoHttps       ProtoType = "https"
	ProtoRediss      ProtoType = "rediss"
	ProtoPostgresqls ProtoType = "postgresqls"
)

var ProtoPipeTypes = map[ProtoType][2]any{
	ProtoHttps:       {nil, &URIHttpsData{}},
	ProtoRediss:      {nil, &URIRedissData{}},
	ProtoPostgresqls: {nil, &URIPostgresqlsData{}},
}

type PipeDefiner interface {
	PipeTypes() [2]any
}

func (a AdapterType) PipeTypes() [2]any {
	return AuthPipeTypes[a]
}
func (p ProtoType) PipeTypes() [2]any {
	return ProtoPipeTypes[p]
}

func getSchemas(provider bool, types []PipeDefiner) (*jsonschema.Schema, *jsonschema.Schema, error) {
	first := []any{}
	second := []any{}
	for _, t := range types {
		both := t.PipeTypes()
		first = append(first, both[0])
		second = append(second, both[1])
	}
	firstSchema, err := generateSchema(first)
	if err != nil {
		return nil, nil, err
	}
	secondSchema, err := generateSchema(second)
	if err != nil {
		return nil, nil, err
	}
	if provider {
		return secondSchema, firstSchema, nil
	}
	return firstSchema, secondSchema, nil
}

func NewTemplate(provider bool, t PipeDefiner, data any) *PipeTemplate {
	thisSchema, otherSchema, err := getSchemas(provider, []PipeDefiner{t})
	if err != nil {
		log.Error(err)
	}
	return &PipeTemplate{
		ID:    t,
		This:  thisSchema,
		Other: otherSchema,
		data:  data,
	}
}

type PipeTemplate struct {
	ID    PipeDefiner        `json:"id"`
	This  *jsonschema.Schema `json:"this,omitempty"`
	Other *jsonschema.Schema `json:"other,omitempty"`
	data  any                `json:"-"`
}

type Blueprint struct {
	Name            string              `json:"name"`
	Adapters        []*PipeTemplate     `json:"adapters"`
	DefaultAdapters []AdapterType       `json:"defaultAdapters"`
	Protos          []*PipeTemplate     `json:"protos"`
	MaxPipes        int                 `json:"maxPipes"`
	pipes           map[string]struct{} `json:"-"`
	mutex           sync.RWMutex        `json:"-"`
}

func NewBlueprint(name string, defaultAdapters []AdapterType, adapters []*PipeTemplate, protos []*PipeTemplate, maxPipes int) *Blueprint {
	return &Blueprint{
		Name:            name,
		Adapters:        adapters,
		DefaultAdapters: defaultAdapters,
		Protos:          protos,
		MaxPipes:        maxPipes,
		pipes:           map[string]struct{}{},
	}
}

func NewNeed(name string, defaultAdapters []AdapterType, adapters []*PipeTemplate, protos []*PipeTemplate) *Blueprint {
	return NewBlueprint(name, defaultAdapters, adapters, protos, 1)
}

func NewOffer(name string, defaultAdapters []AdapterType, adapters []*PipeTemplate, protos []*PipeTemplate) *Blueprint {
	return NewBlueprint(name, defaultAdapters, adapters, protos, 0)
}

type Binding struct {
	Pipe
	Adapters []AdapterType `json:"adapters"`
	Proto    ProtoType     `json:"proto"`
}

func (s *Blueprint) AddPipe(id string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.MaxPipes != 0 && len(s.pipes) >= s.MaxPipes {
		return false
	}
	if _, ok := s.pipes[id]; ok {
		// this should not happen
		panic("Sync failure")
	}

	s.pipes[id] = struct{}{}
	return true
}

func (s *Blueprint) DeletePipe(id string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.pipes, id)
}

type End struct {
	Issuer string             `json:"issuer,omitempty"`
	URI    string             `json:"uri,omitempty"`
	Schema *jsonschema.Schema `json:"schema,omitempty"`
	Data   json.RawMessage    `json:"data,omitempty"`
}

func (e *End) Equals(other End) bool {
	return e.Issuer == other.Issuer &&
		e.URI == other.URI &&
		(e.Schema == other.Schema || (e.Schema != nil && other.Schema != nil && e.Schema.ID == other.Schema.ID)) &&
		bytes.Equal(e.Data, other.Data)
}

const schemaVersion = "https://json-schema.org/draft/2020-12/schema"

func generateSchema(items []any) (*jsonschema.Schema, error) {
	reflector := &jsonschema.Reflector{
		ExpandedStruct: true,
	}
	schemas := []*jsonschema.Schema{}
	for _, item := range items {
		if item == nil {
			continue
		}
		s := reflector.Reflect(item)
		s.AdditionalProperties = nil
		schemas = append(schemas, s)
	}
	return combineSchemas(schemas)
}

func combineSchemas(items []*jsonschema.Schema) (*jsonschema.Schema, error) {
	filtered := []*jsonschema.Schema{}
	for _, item := range items {
		if item == nil {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	var schema *jsonschema.Schema

	if len(filtered) == 1 {
		schema = filtered[0]
	} else {
		log.Errorf("Multiple Items %+v", filtered)
		for _, item := range filtered {
			item.Version = ""
		}
		schema = &jsonschema.Schema{
			Version: schemaVersion,
			AllOf:   filtered,
		}
	}
	return schema, nil
}

func (e *End) SetSchema(items []any) error {
	var err error
	e.Schema, err = generateSchema(items)
	return err
}

func (e *End) SetData(d any) error {
	oldData := map[string]interface{}{}
	if !isJSONEmpty(e.Data) {
		if err := json.Unmarshal(e.Data, &oldData); err != nil {
			return err
		}
	}

	newDataJSON, err := json.Marshal(d)
	if err != nil {
		return err
	}

	var newData map[string]interface{}
	if err := json.Unmarshal(newDataJSON, &newData); err != nil {
		return err
	}

	for key, value := range newData {
		oldData[key] = value
	}

	modified, err := json.Marshal(oldData)
	if err != nil {
		return err
	}
	e.Data = json.RawMessage(modified)
	return nil
}

func (e *End) Validate() error {
	if e.Schema == nil || isJSONEmpty(e.Data) {
		return nil
	}

	// Marshal e.Schema to JSON bytes
	schemaBytes, err := json.Marshal(e.Schema)
	if err != nil {
		return err
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	dataLoader := gojsonschema.NewBytesLoader(e.Data)

	result, err := gojsonschema.Validate(schemaLoader, dataLoader)
	if err != nil {
		return err
	}

	if !result.Valid() {
		s := "Data does not match schema. see errors :\n"
		for _, desc := range result.Errors() {
			s = fmt.Sprintf("%s- %s\n", s, desc)
		}
		return fmt.Errorf("%s", s)
	}
	return nil
}

func (e *End) Merge(o *End) error {
	var err error
	e.Schema, err = combineSchemas([]*jsonschema.Schema{e.Schema, o.Schema})
	if err != nil {
		return err
	}
	e.Data, err = mergeJSONRawMessages(e.Data, o.Data)
	if err != nil {
		return err
	}
	return nil
}

type Link struct {
	Href string `json:"href"`
	// Title     string `json:"title,omitempty"`
	// Templated bool   `json:"templated,omitempty"`
}

type Links struct {
	Self      *Link   `json:"self,omitempty"`
	Blueprint *Link   `json:"blueprint,omitempty"`
	Adapters  []*Link `json:"adapters,omitempty"`
	Proto     *Link   `json:"proto,omitempty"`
}

type Pipe struct {
	ID        string     `json:"id,omitempty"`
	This      End        `json:"this,omitempty"`
	Other     End        `json:"other,omitempty"`
	Links     Links      `json:"_links"`
	blueprint *Blueprint `json:"-"`
}

func (p *Pipe) Validate() error {
	if err := p.This.Validate(); err != nil {
		return err
	}
	if err := p.Other.Validate(); err != nil {
		return err
	}
	return nil
}

func (p *Pipe) Merge(o *Pipe) error {
	if err := p.This.Merge(&o.This); err != nil {
		return err
	}
	if err := p.Other.Merge(&o.Other); err != nil {
		return err
	}
	return nil
}

func isJSONEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	return false
}

func mergeJSONRawMessages(a, b json.RawMessage) (json.RawMessage, error) {
	if isJSONEmpty(a) {
		return b, nil
	}
	if isJSONEmpty(b) {
		return a, nil
	}

	var mapA, mapB map[string]interface{}

	if err := json.Unmarshal(a, &mapA); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(b, &mapB); err != nil {
		return nil, err
	}

	for key, value := range mapB {
		mapA[key] = value
	}

	merged, err := json.Marshal(mapA)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(merged), nil
}

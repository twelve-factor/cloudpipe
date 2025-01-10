package cmd

import (
	// "encoding/json"

	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// providerCmd run provider server
var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Provider broker",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("invalid command")
		}
		return runProvider()
	},
}

func init() {
	cmd.AddCommand(providerCmd)
}

func runProvider() error {
	resources := map[string]*Resource{}
	pgdata := URIPostgresqlsData{
		URI: "postgresql://user:password@db.example.com:5432/mydb",
	}
	resources["db"] = &Resource{
		ID:          "db",
		Pipes:       map[string]*Pipe{},
		DefaultData: pgdata,
		Offers: []*Blueprint{
			NewOffer(
				"postgresqls",
				nil,
				[]*PipeTemplate{
					NewTemplate(
						true,
						ServerAuth,
						nil,
					),
				},
				[]*PipeTemplate{
					NewTemplate(
						true,
						ProtoPostgresqls,
						pgdata,
					),
				},
			),
		},
		Needs: []*Blueprint{},
	}
	resources["backend"] = &Resource{
		ID:    "backend",
		Pipes: map[string]*Pipe{},
		Offers: []*Blueprint{
			NewOffer(
				"https",
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
							URI: "https://backend.herokuapp.com",
						},
					),
				},
			),
		},
		Needs: []*Blueprint{},
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINFO)

	go func() {
		_, prefix := getPortAndPrefix("8001")
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
	return runBrokerServer("8001", resources)
}

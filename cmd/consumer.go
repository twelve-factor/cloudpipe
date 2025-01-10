package cmd

import (
	// "encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// consumerCmd run consumer server
var consumerCmd = &cobra.Command{
	Use:   "consumer",
	Short: "Consumer broker",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("invalid command")
		}
		return runConsumer()
	},
}

func init() {
	cmd.AddCommand(consumerCmd)
}

func runConsumer() error {
	resources := map[string]*Resource{}
	resources["frontend"] = &Resource{
		ID:     "frontend",
		Pipes:  map[string]*Pipe{},
		Offers: []*Blueprint{},
		Needs: []*Blueprint{
			NewNeed(
				"db",
				nil,
				[]*PipeTemplate{
					NewTemplate(
						false,
						OIDCAuth,
						OIDCAuthData{
							Issuer:   "https://oidc.heroku.com",
							Subject:  "frontend",
							Audience: "db",
						},
					),
					NewTemplate(
						false,
						ServerAuth,
						nil,
					),
				},
				[]*PipeTemplate{
					NewTemplate(
						false,
						ProtoPostgresqls,
						nil,
					),
				},
			),
			NewNeed(
				"backend",
				[]AdapterType{OIDCAuth},
				[]*PipeTemplate{
					NewTemplate(
						false,
						OIDCAuth,
						OIDCAuthData{
							Issuer:   "https://oidc.heroku.com",
							Subject:  "frontend",
							Audience: "backend",
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
	return runBrokerServer("8000", resources)
}

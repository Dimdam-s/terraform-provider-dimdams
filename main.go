// Terraform provider for Dimdam's: manages the configuration of existing services (firewall, alerts,
// environment variables, SSH keys, scheduled tasks, custom domain) through the public REST API.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/dimdams/terraform-provider-dimdams/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

//go:generate go tool tfplugindocs generate --provider-name dimdams

// Set by the release build.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/dimdams/dimdams",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/jackyoung/terraform-provider-zarf/internal/provider"
)

var version string = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run provider with support for debuggers")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/jackyoung/zarf",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err)
	}
}

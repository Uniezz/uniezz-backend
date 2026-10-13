// Command openapi prints the OpenAPI spec of the Uniezz API. It registers the
// operations without handlers, so it needs no database, .env or USOS credentials.
package main

import (
	"flag"
	"io"
	"log"
	"os"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/server"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatalf("openapi: %v", err)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("openapi", flag.ContinueOnError)
	out := fs.String("o", "", "write the spec to this file instead of stdout")
	fs.StringVar(&httpapi.Version, "version", httpapi.Version, "value of info.version")
	if err := fs.Parse(args); err != nil {
		return err
	}

	spec, err := buildSpec()
	if err != nil {
		return err
	}

	if *out == "" {
		_, err = stdout.Write(spec)
		return err
	}
	return os.WriteFile(*out, spec, 0o644)
}

func buildSpec() ([]byte, error) {
	api := humachi.New(chi.NewMux(), httpapi.NewConfig(&config.Config{}))
	server.Modules{}.Register(api)
	return api.OpenAPI().YAML()
}

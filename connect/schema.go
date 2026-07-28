package connect

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/panaudia/lasa/schemata"
)

var compileOnce sync.Once
var compiled struct {
	connection *jsonschema.Schema
	ticket     *jsonschema.Schema
	err        error
}

func schemas() (connection, ticket *jsonschema.Schema, err error) {
	compileOnce.Do(func() {
		c := jsonschema.NewCompiler()
		for name, url := range map[string]string{
			"connection.schema.json": schemata.ConnectionSchemaURL,
			"entity.schema.json":     schemata.EntitySchemaURL,
			"setup.schema.json":      schemata.SetupSchemaURL,
			"pose.schema.json":       schemata.PoseSchemaURL,
			"ticket.schema.json":     schemata.TicketSchemaURL,
		} {
			raw, err := schemata.FS.ReadFile(name)
			if err != nil {
				compiled.err = err
				return
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				compiled.err = fmt.Errorf("schema %s: %w", name, err)
				return
			}
			if err := c.AddResource(url, doc); err != nil {
				compiled.err = fmt.Errorf("schema %s: %w", name, err)
				return
			}
		}
		if compiled.connection, compiled.err = c.Compile(schemata.ConnectionSchemaURL); compiled.err != nil {
			return
		}
		compiled.ticket, compiled.err = c.Compile(schemata.TicketSchemaURL)
	})
	return compiled.connection, compiled.ticket, compiled.err
}

// validateAgainst validates raw JSON bytes against a compiled schema.
func validateAgainst(schema *jsonschema.Schema, raw []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}

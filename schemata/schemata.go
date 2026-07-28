// Package schemata embeds the normative LASA JSON schemas so Go consumers
// can validate against exactly the files published in this repository.
package schemata

import "embed"

// FS holds the five schema files: connection, entity, setup, pose, ticket.
//
//go:embed *.json
var FS embed.FS

// The $id URL of each schema, as referenced by cross-schema $refs.
const (
	ConnectionSchemaURL = "https://panaudia.com/schemata/connection.schema.json"
	EntitySchemaURL     = "https://panaudia.com/schemata/entity.schema.json"
	SetupSchemaURL      = "https://panaudia.com/schemata/setup.schema.json"
	PoseSchemaURL       = "https://panaudia.com/schemata/pose.schema.json"
	TicketSchemaURL     = "https://panaudia.com/schemata/ticket.schema.json"
)

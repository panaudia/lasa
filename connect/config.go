package connect

import "encoding/json"

// ConnectionConfig is the message a client presents exactly once, at MoQ
// session establishment, as the SetupParamConnectionConfig CLIENT_SETUP
// parameter (lasa-core.md §4.1). Validated against
// schemata/connection.schema.json.
type ConnectionConfig struct {
	ClientID string   `json:"client_id"`
	Ticket   string   `json:"ticket,omitempty"`
	Entities []Entity `json:"entities,omitempty"`
	Setups   []Setup  `json:"setups,omitempty"`

	hasEntities bool
	hasSetups   bool
}

// HasEntities reports whether the entities key was present in the config —
// an explicitly empty list is the legal zero-entity observer pattern
// (lasa-core.md §4.2), distinct from the key being absent.
func (c *ConnectionConfig) HasEntities() bool { return c.hasEntities }

// HasSetups reports whether the setups key was present in the config.
func (c *ConnectionConfig) HasSetups() bool { return c.hasSetups }

// Entity is an entity definition (schemata/entity.schema.json), appearing
// either ad-hoc in a Connection Config or signed inside a Ticket.
type Entity struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Priority    bool           `json:"priority,omitempty"`
	HeardIn     []string       `json:"heard-in,omitempty"`
	Hears       []string       `json:"hears,omitempty"`
	Attrs       map[string]any `json:"attrs,omitempty"`
	Frame       string         `json:"frame,omitempty"`
	Dof         *int           `json:"dof,omitempty"`
	Quality     int            `json:"quality,omitempty"`
	Redundancy  int            `json:"redundancy,omitempty"`
	InitialPose *PoseSpec      `json:"initial-pose,omitempty"`
	Render      *Render        `json:"render,omitempty"`
}

// Setup instantiates a ticket-defined entity by id, supplying the
// runtime-only parts: an initial pose overriding the definition's default,
// and unsigned attrs (lasa-core.md §4.2).
type Setup struct {
	ID          string         `json:"id"`
	Attrs       map[string]any `json:"attrs,omitempty"`
	InitialPose *PoseSpec      `json:"initial-pose,omitempty"`
}

// PoseSpec is the JSON pose of schemata/pose.schema.json (engineering
// units; the wire pose of package wire is its binary counterpart).
type PoseSpec struct {
	Position *Position `json:"position,omitempty"`
	Attitude *Attitude `json:"attitude,omitempty"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Attitude struct {
	Yaw   float64 `json:"yaw"`
	Pitch float64 `json:"pitch"`
	Roll  float64 `json:"roll"`
}

// Render carries the render.* defaults from an entity definition
// (base-profile vocabulary; nil field = absent = documented default).
type Render struct {
	Gain        *float64 `json:"gain,omitempty"`
	Attenuation *float64 `json:"attenuation,omitempty"`
	Size        *float64 `json:"size,omitempty"`
	Directivity *float64 `json:"directivity,omitempty"`
}

// ParseConnectionConfig validates raw against the connection schema and
// decodes it. Schema violations return a Rejection with code
// ErrCodeMalformedConfig.
func ParseConnectionConfig(raw []byte) (*ConnectionConfig, *Rejection) {
	connSchema, _, err := schemas()
	if err != nil {
		return nil, rejectMalformed("internal: schemas unavailable: %v", err)
	}
	if err := validateAgainst(connSchema, raw); err != nil {
		return nil, rejectMalformed("connection config: %v", err)
	}
	var cfg ConnectionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, rejectMalformed("connection config: %v", err)
	}
	var presence struct {
		Entities json.RawMessage `json:"entities"`
		Setups   json.RawMessage `json:"setups"`
	}
	if err := json.Unmarshal(raw, &presence); err != nil {
		return nil, rejectMalformed("connection config: %v", err)
	}
	cfg.hasEntities = presence.Entities != nil
	cfg.hasSetups = presence.Setups != nil
	return &cfg, nil
}

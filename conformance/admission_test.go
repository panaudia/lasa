package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestRejectionCodes drives every connect-time refusal the polite
// client can produce and checks the §4.6 code on the wire. Rows that
// need a ticket run on ticketed fixtures and the others on unticketed
// ones. The hostile-client layer will add the malformed-config rows.
func TestRejectionCodes(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		one := []connect.Entity{{ID: "rej-e", Name: "A"}}

		// Both modes: a config with neither entities nor setups.
		cfg := configFor(t, f, party{clientID: "rej-none", entities: one})
		cfg.Entities, cfg.Setups = nil, nil
		expectRejection(t, "C-4.1-3", f, cfg, wire.ErrCodeMalformedConfig)
		Pass(t, "C-4.1-2", "a schema-invalid config rejects with 0x1A5A00 (neither entities nor setups)")
		Pass(t, "C-4.1-4", "rejection arrived as a session close before setup completed")
		Pass(t, "C-4.6-1", "code and reason carried on %s", f.Transport)

		if !f.Ticketed {
			// A ticket into an unticketed space, whoever signed it.
			tok := mint(t, f, otherKey(t), connect.MintClaims{Issuer: "x", SpaceID: f.SpaceID, ClientID: "rej-tok", Entities: one, IssuedAt: time.Now()})
			expectRejection(t, "C-4.4-2", f, configFor(t, f, party{clientID: "rej-tok", entities: one, ticket: tok}), wire.ErrCodeModeMismatch)
			for _, id := range []string{"C-4.4-1", "C-4.3-1", "C-4.3-3", "C-4.3-6", "C-4.3-7", "C-2-5", "C-4.2-1", "C-4.2-3", "C-4.4-4", "P-2-2"} {
				NotExercised(t, id, "unticketed fixture")
			}
			return
		}
		NotExercised(t, "C-4.4-2", "ticketed fixture")

		// No ticket at all.
		cfg = configFor(t, f, party{clientID: "rej-noticket", entities: one})
		cfg.Ticket, cfg.Setups, cfg.Entities = "", nil, one
		expectRejection(t, "C-4.4-1", f, cfg, wire.ErrCodeInvalidTicket)

		claims := func(clientID string) connect.MintClaims {
			return connect.MintClaims{Issuer: f.Issuer, SpaceID: f.SpaceID, ClientID: clientID, Entities: one, IssuedAt: time.Now()}
		}
		// Wrong signer.
		expectRejection(t, "C-4.3-1", f, configFor(t, f, party{clientID: "rej-sig", entities: one, ticket: mint(t, f, otherKey(t), claims("rej-sig"))}), wire.ErrCodeInvalidTicket)
		// Expired, and not yet valid.
		past := time.Now().Add(-time.Hour)
		c := claims("rej-exp")
		c.ExpiresAt = &past
		expectRejection(t, "C-4.3-3", f, configFor(t, f, party{clientID: "rej-exp", entities: one, ticket: mint(t, f, f.IssuerKey, c)}), wire.ErrCodeInvalidTicket)
		future := time.Now().Add(time.Hour)
		c = claims("rej-nbf")
		c.NotBefore = &future
		expectRejection(t, "C-4.3-3", f, configFor(t, f, party{clientID: "rej-nbf", entities: one, ticket: mint(t, f, f.IssuerKey, c)}), wire.ErrCodeInvalidTicket)
		// Wrong audience.
		c = claims("rej-aud")
		c.SpaceID = f.SpaceID + "-other"
		expectRejection(t, "C-2-5", f, configFor(t, f, party{clientID: "rej-aud", entities: one, ticket: mint(t, f, f.IssuerKey, c)}), wire.ErrCodeInvalidTicket)
		// client_id claim differs from the config's.
		expectRejection(t, "C-4.3-6", f, configFor(t, f, party{clientID: "rej-mismatch", entities: one, ticket: mint(t, f, f.IssuerKey, claims("someone-else"))}), wire.ErrCodeClientIDMismatch)
		// Unknown role name.
		c = claims("rej-role")
		c.Roles = []string{"wizard"}
		expectRejection(t, "P-2-2", f, configFor(t, f, party{clientID: "rej-role", entities: one, ticket: mint(t, f, f.IssuerKey, c)}), wire.ErrCodeInvalidTicket)
		Pass(t, "C-4.3-7", "unknown role name refused with 0x1A5A01 per the base profile")
		// Ad-hoc entities on an audience ticket.
		cfg = configFor(t, f, party{clientID: "rej-adhoc", entities: []connect.Entity{}})
		cfg.Entities, cfg.Setups = one, nil
		expectRejection(t, "C-4.2-3", f, cfg, wire.ErrCodeAdHocNotPermitted)
		// A setup naming an entity the ticket does not define.
		cfg = configFor(t, f, party{clientID: "rej-setup", entities: one})
		cfg.Setups = []connect.Setup{{ID: "not-in-ticket"}}
		expectRejection(t, "C-4.2-1", f, cfg, wire.ErrCodeMalformedConfig)

		// Cross-owner entity collision: c1 holds rej-shared, c2's ticket
		// also defines it. c2 is refused and c1 is never severed.
		shared := []connect.Entity{{ID: "rej-shared", Name: "S"}}
		c1 := mustDial(t, f, party{clientID: "rej-col-1", entities: shared})
		expectRejection(t, "C-4.4-4", f, configFor(t, f, party{clientID: "rej-col-2", entities: shared}), wire.ErrCodeEntityCollision)
		select {
		case <-c1.Done():
			Fail(t, "C-4.4-4", "the bystander was severed by the collision: %v", c1.Err())
		case <-time.After(300 * time.Millisecond):
		}
	})
}

// TestSupersession covers C-4.4-3 and C-4.4-10 (ticketed), C-4.4-5
// (unticketed) and the C-4.6-7 code on a superseded session.
func TestSupersession(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		one := []connect.Entity{{ID: "sup-e", Name: "A"}}
		first := mustDial(t, f, party{clientID: "sup-c", entities: one})
		_ = mustDial(t, f, party{clientID: "sup-c", entities: one})
		id := "C-4.4-3"
		if !f.Ticketed {
			id = "C-4.4-5"
		}
		expectClosed(t, id, first, wire.ErrCodeSuperseded)
		Pass(t, "C-4.6-7", "supersession closed the established session with 0x1A5A0A on %s", f.Transport)

		// Zero-entity connections exercise the client capability too.
		obs := mustDial(t, f, party{clientID: "sup-obs", entities: []connect.Entity{}})
		_ = mustDial(t, f, party{clientID: "sup-obs", entities: []connect.Entity{}})
		expectClosed(t, "C-4.4-10", obs, wire.ErrCodeSuperseded)

		if !f.Ticketed {
			// Any collision supersedes: a different client id claiming a
			// live entity evicts its holder.
			holder := mustDial(t, f, party{clientID: "sup-h1", entities: []connect.Entity{{ID: "sup-shared", Name: "A"}}})
			_ = mustDial(t, f, party{clientID: "sup-h2", entities: []connect.Entity{{ID: "sup-shared", Name: "B"}}})
			expectClosed(t, "C-4.4-5", holder, wire.ErrCodeSuperseded)
		}
	})
}

// TestObserver covers C-4.2-6: a zero-entity connection is admitted
// and may subscribe presence and its own state sink.
func TestObserver(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c, _ := observer(t, f, "obs-c", []string{""}, connect.Permissions{})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		_, rt, err := c.SubscribeRoster(ctx)
		if err == nil {
			defer rt.Close()
		}
		Check(t, "C-4.2-6", err == nil, "observer could not subscribe presence: %v", err)
	})
}

// TestShutdownCode covers the shutdown half of C-4.6-7 on a private
// reference, since a shared or external fixture cannot be stopped.
func TestShutdownCode(t *testing.T) {
	if externalOnly() {
		NotExercised(t, "C-4.6-7", "shutdown: external fixture provides no shutdown hook")
		return
	}
	for _, tr := range []Transport{TransportQUIC, TransportWebTransport} {
		t.Run(string(tr), func(t *testing.T) {
			f := privateReference(t, tr, false, nil)
			c := mustDial(t, f, party{clientID: "shut-c", entities: []connect.Entity{{ID: "shut-e", Name: "A"}}})
			ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
			defer cancel()
			if err := f.Shutdown(ctx); err != nil {
				t.Fatalf("shutdown: %v", err)
			}
			expectClosed(t, "C-4.6-7", c, wire.ErrCodeShuttingDown)
		})
	}
}

// Package connect implements LASA connection establishment (lasa-core.md
// §4): parsing and validating the Connection Config against the normative
// schemas, verifying Tickets (Ed25519 JWTs), resolving the config's
// entities and setups into the connection's admitted entity set, and the
// §4.6 rejection codes.
//
// The package is deliberately below any transport: the server shell feeds
// it the CLIENT_SETUP parameter bytes and turns a *Rejection into a
// session close with the carried code and reason.
package connect

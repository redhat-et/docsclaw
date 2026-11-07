// Package a2acall implements the a2a_call tool, which lets the LLM invoke a
// configured peer agent over A2A. The tool sends no credentials: in an
// OpenShell sandbox the supervisor injects them at the network boundary.
//
// # Signed agent cards
//
// A peer may configure card_public_key, a PEM-encoded public key. Before the
// first call to that peer, a2a_call fetches
// <peer origin>/.well-known/agent-card.json and verifies one of the card's
// detached JWS signatures.
//
// The wire format is the A2A AgentCard JSON with a "signatures" array of
// AgentCardSignature entries. Each entry carries a base64url "protected"
// header and a base64url "signature". The protected header must contain:
//
//   - alg: ES256 (P-256 key) or RS256 (RSA key), matching the configured key
//   - typ: JOSE
//   - kid: non-empty; the pinned per-peer key still decides which key verifies
//
// Per the A2A specification, the signed payload is the RFC 8785 (JCS)
// canonical form of the card with the "signatures" field removed. Optional
// fields that were explicitly set are kept, so verification tries several
// candidate payloads and succeeds if a signature covers any of them:
//
//  1. the card exactly as served;
//  2. empty strings, arrays, and objects removed, but explicit false booleans
//     kept — the form in the specification's worked example, where an empty
//     capabilities.extensions array is dropped while explicitly-set
//     streaming/pushNotifications false flags remain;
//  3. all default values removed too (false booleans, zero numbers) — the form
//     a signer produces when it strips every default-valued property;
//  4. the same as 3 but also removing required properties that arrive empty,
//     for servers whose struct marshaling re-emits capabilities, skills, or
//     modes the signer never set.
//
// The default-valued optional properties this covers are, in practice:
// documentationUrl, iconUrl, provider, securityRequirements, securitySchemes,
// and supportsAuthenticatedExtendedCard at the top level, plus streaming,
// pushNotifications, stateTransitionHistory, extensions, and
// extendedAgentCard inside capabilities.
//
// Required properties are kept even when empty in forms 1-3: name,
// description, version, capabilities, skills, supportedInterfaces,
// defaultInputModes, and defaultOutputModes. Trying several forms cannot
// weaken the binding: a signature must still verify over the exact bytes that
// were signed.
package a2acall

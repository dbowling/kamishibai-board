package domain

// Embed the IANA timezone database in the binary.
//
// Period boundaries are evaluated in a named location (America/New_York by
// default). Without this import, time.LoadLocation depends on the host having
// system tzdata installed, which a minimal container image will not. Embedding
// it costs a few hundred KB and removes an entire class of "works on my Mac,
// silently computes UTC boundaries in Kubernetes" bug.
import _ "time/tzdata"

package embed

// The two failures of a backend say what they are in the terms the consumers of a client own: internal/query
// finds a model that is not pulled and a backend that did not answer by these methods (query.EmbedModelMissing
// and query.EmbedBackendUnreachable), and the diagnostics of internal/ops find a model that is not pulled by the
// first (ops.EmbedModelMissing; every other failure is a backend that did not answer), however the error is
// wrapped, so neither imports this package to tell them apart. The methods carry no behavior; their presence is
// the statement.

// EmbedModelMissing says the backend answered but the model is not pulled (R-070).
func (*ModelMissingError) EmbedModelMissing() {}

// EmbedBackendUnreachable says the backend never answered at all (R-070).
func (*BackendUnreachableError) EmbedBackendUnreachable() {}

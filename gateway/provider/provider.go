package provider

import "context"

// TryOnRequest is provider-agnostic input for one virtual try-on job.
type TryOnRequest struct {
	// PersonImage and GarmentImage are either "data:image/...;base64,..."
	// data URIs or publicly reachable https:// URLs.
	PersonImage  string
	GarmentImage string
	// Category: "tops" | "bottoms" | "one-pieces" | "auto"
	Category string
}

// Provider submits try-on jobs to an upstream API and polls them.
// Implementations must be safe for concurrent use.
type Provider interface {
	Name() string
	// Submit starts a job and returns the provider-side task/request id.
	Submit(ctx context.Context, req TryOnRequest) (providerTaskID string, err error)
	// Poll checks one job. done=true means terminal; resultURL is set on success.
	Poll(ctx context.Context, providerTaskID string) (done bool, resultURL string, err error)
}

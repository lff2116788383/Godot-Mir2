// Package version exposes build identity for the server binary.
package version

const (
	// Product is the user-facing executable identity.
	Product = "mir2x-server-go"

	// Version is replaced by the release build when required.
	Version = "0.1.0-dev"
)

// Full returns the product name and version in a single string.
func Full() string {
	return Product + " " + Version
}

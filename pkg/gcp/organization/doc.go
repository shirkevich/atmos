// Package organization provides GCP organization retrieval and caching.
//
// This package consolidates GCP organization-related functionality used by Atmos functions
// (YAML, HCL, etc.) and provides a clean, reusable interface for organization operations.
//
// Key features:
//   - GCP organization lookup via Cloud Resource Manager project ancestry API
//   - Thread-safe caching of organization results per project ID
//   - Testable via Getter interface and injectable client factory
//   - Support for multiple GCP authentication methods (access token, credentials file, ADC)
package organization

// Package identity provides GCP caller identity resolution with caching.
// It reads identity information (project ID, region, service account email)
// from the Atmos GCP auth context, environment variables, and Application
// Default Credentials (ADC) as fallbacks.
package identity

package validation

import (
	"fmt"

	"github.com/nstalgic/nekzus/internal/types"
)

// ValidateApp validates an App structure
func ValidateApp(app types.App) error {
	if err := ValidateID(app.ID); err != nil {
		return fmt.Errorf("invalid app ID: %w", err)
	}

	if err := ValidateName(app.Name, 200); err != nil {
		return fmt.Errorf("invalid app name: %w", err)
	}

	// Icon is optional, but if provided should be validated
	if app.Icon != "" {
		if err := ValidateName(app.Icon, 100); err != nil {
			return fmt.Errorf("invalid app icon: %w", err)
		}
	}

	// Validate tags if present
	for _, tag := range app.Tags {
		if err := ValidateName(tag, 50); err != nil {
			return fmt.Errorf("invalid tag %q: %w", tag, err)
		}
	}

	return nil
}

// ValidateRoute validates a Route structure
func ValidateRoute(route types.Route) error {
	if err := ValidateID(route.RouteID); err != nil {
		return fmt.Errorf("invalid route ID: %w", err)
	}

	if err := ValidateID(route.AppID); err != nil {
		return fmt.Errorf("invalid app ID: %w", err)
	}

	if err := ValidatePath(route.PathBase); err != nil {
		return fmt.Errorf("invalid path base: %w", err)
	}

	if err := ValidateURL(route.To); err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	// Validate scopes
	for _, scope := range route.Scopes {
		if err := ValidateScope(scope); err != nil {
			return fmt.Errorf("invalid scope %q: %w", scope, err)
		}
	}

	return nil
}

// ValidateDeviceRegistration validates device registration request
func ValidateDeviceRegistration(deviceID, platform string) error {
	if err := ValidateID(deviceID); err != nil {
		return fmt.Errorf("invalid device ID: %w", err)
	}

	if err := ValidatePlatform(platform); err != nil {
		return fmt.Errorf("invalid platform: %w", err)
	}

	return nil
}

// ValidateAPIKey validates an API key structure
func ValidateAPIKey(key types.APIKey) error {
	if err := ValidateID(key.ID); err != nil {
		return fmt.Errorf("invalid API key ID: %w", err)
	}

	if err := ValidateName(key.Name, 100); err != nil {
		return fmt.Errorf("invalid API key name: %w", err)
	}

	// Validate scopes
	for _, scope := range key.Scopes {
		if err := ValidateScope(scope); err != nil {
			return fmt.Errorf("invalid scope %q: %w", scope, err)
		}
	}

	return nil
}

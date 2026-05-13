package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// timeFormat is RFC 3339 with a numeric timezone offset. It matches what
// the Datadog API returns for created_at / updated_at fields after
// parsing and avoids state churn from format differences.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// apiErr formats an SDK error along with the HTTP status code when one
// is available.
func apiErr(err error, httpResp *http.Response) string {
	if httpResp == nil {
		return err.Error()
	}
	return fmt.Sprintf("%s (status %d)", err.Error(), httpResp.StatusCode)
}

// errNotFound is returned when a resource was looked up by ID but the
// server reported it absent. Callers translate this into State.RemoveResource.
var errNotFound = errors.New("resource not found")

// isNotFound reports whether err originated from a 404 response.
func isNotFound(err error) bool { return errors.Is(err, errNotFound) }

// notFoundIfHTTP404 converts an SDK error + HTTP response pair into
// errNotFound when the server returned 404. Use this immediately after
// SDK calls so downstream `isNotFound(err)` checks light up cleanly.
func notFoundIfHTTP404(err error, httpResp *http.Response) error {
	if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	return err
}

// parseFlagEnvIDs validates and parses the (feature_flag_id, environment_id)
// pair used by composite resources like ddff_feature_flag_environment and
// ddff_allocation_set.
func parseFlagEnvIDs(flag, env string, diags *diag.Diagnostics) (uuid.UUID, uuid.UUID, bool) {
	flagID, ok := parseUUID(flag, "feature_flag_id", diags)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	envID, ok := parseUUID(env, "environment_id", diags)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return flagID, envID, true
}

// parseUUID validates a single UUID string, appending a typed error to
// diags when it does not parse.
func parseUUID(s, fieldName string, diags *diag.Diagnostics) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		diags.AddError("Invalid "+fieldName, err.Error())
		return uuid.Nil, false
	}
	return id, true
}

// configureClients implements the boilerplate every resource and data
// source repeats inside its Configure callback: it pulls *Clients out
// of ProviderData, appending a typed diag on the rare miss. Returns
// nil when ProviderData is unset (early provider phase) so the caller
// can no-op without surfacing an error.
func configureClients(providerData interface{}, diags *diag.Diagnostics) *Clients {
	if providerData == nil {
		return nil
	}
	clients, ok := providerData.(*Clients)
	if !ok {
		diags.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Clients, got %T", providerData),
		)
		return nil
	}
	return clients
}

// nullableTimeToTF converts an SDK *time.Time into a framework
// types.String formatted with timeFormat, returning null when the
// pointer is nil.
func nullableTimeToTF(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(timeFormat))
}

// nullableStringToTF converts an SDK datadog.NullableString into a
// framework types.String, mapping "not set" and "explicit null" both
// to types.StringNull().
func nullableStringToTF(v datadog.NullableString) types.String {
	if !v.IsSet() || v.Get() == nil {
		return types.StringNull()
	}
	return types.StringValue(*v.Get())
}

// findRawEnvEntry fetches the parent feature flag and locates the env
// entry inside feature_flag_environments[] matching envID. The lookup
// is done against the raw JSON map (recovered from UnparsedObject when
// strict typed unmarshal already failed, or via round-trip marshal
// otherwise) to side-step the SDK regression where allocations[] is
// declared as a map but returned as an array, collapsing the typed
// entry into UnparsedObject as soon as any allocation exists.
//
// Returns errNotFound when the flag itself or the env entry is absent.
func (c *Clients) findRawEnvEntry(ctx context.Context, flagID, envID uuid.UUID) (map[string]interface{}, error) {
	res, httpResp, err := c.FeatureFlags.GetFeatureFlag(c.Context(ctx), flagID)
	if err != nil {
		return nil, notFoundIfHTTP404(err, httpResp)
	}

	wantID := envID.String()
	for _, env := range res.Data.Attributes.FeatureFlagEnvironments {
		raw, ok := envEntryToRaw(env)
		if !ok {
			continue
		}
		envIDInJSON, _ := raw["environment_id"].(string)
		if envIDInJSON == wantID {
			return raw, nil
		}
	}
	return nil, errNotFound
}

// envEntryToRaw recovers a generic map representation of a single
// feature_flag_environments[] entry, preferring the raw JSON captured
// in UnparsedObject (which is the shape the server actually sent) and
// falling back to a typed-marshal round trip when the SDK accepted the
// entry cleanly (no allocations present).
func envEntryToRaw(env datadogV2.FeatureFlagEnvironment) (map[string]interface{}, bool) {
	if env.UnparsedObject != nil {
		return env.UnparsedObject, true
	}
	bytes, err := env.MarshalJSON()
	if err != nil {
		return nil, false
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, false
	}
	return raw, true
}

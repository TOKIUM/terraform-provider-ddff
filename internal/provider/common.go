package provider

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// parseFlagEnvIDs validates and parses the (feature_flag_id, environment_id)
// pair used by composite resources like ddff_feature_flag_environment and
// ddff_allocation.
func parseFlagEnvIDs(flag, env string, diags *diag.Diagnostics) (uuid.UUID, uuid.UUID, bool) {
	flagID, err := uuid.Parse(flag)
	if err != nil {
		diags.AddError("Invalid feature_flag_id", err.Error())
		return uuid.Nil, uuid.Nil, false
	}
	envID, err := uuid.Parse(env)
	if err != nil {
		diags.AddError("Invalid environment_id", err.Error())
		return uuid.Nil, uuid.Nil, false
	}
	return flagID, envID, true
}

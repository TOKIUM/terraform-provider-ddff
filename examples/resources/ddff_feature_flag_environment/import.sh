#!/usr/bin/env bash
# Import is keyed by "<feature_flag_id>:<environment_id>". Replace both
# UUIDs below with the actual IDs from the Datadog UI or API.
terraform import \
  ddff_feature_flag_environment.new_checkout_prod \
  00000000-0000-0000-0000-000000000000:00000000-0000-0000-0000-000000000001

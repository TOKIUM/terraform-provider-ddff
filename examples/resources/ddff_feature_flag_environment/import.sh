#!/usr/bin/env bash
# Import is keyed by "<feature_flag_id>:<environment_id>".
terraform import \
  ddff_feature_flag_environment.new_checkout_prod \
  4f8e31e1-307f-4a70-a245-9b2426d25415:0a9a8d89-6fab-4829-9c72-b33540fb8fce

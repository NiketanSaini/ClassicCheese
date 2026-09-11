package main

import "errors"

var (
	errRequiredFields = errors.New("user_id, region and score_delta are required")
	errMissingRegion  = errors.New("region is required for this scope")
	errMissingUserID  = errors.New("user_id is required")
	errUnknownScope   = errors.New("scope must be one of: global, regional, friends")
)

package main

import "errors"

// errEmptyInput is shared by every Actor built from this scaffold: an alias field's entry that
// trims to nothing. Add niche-specific errors (e.g. "no hostname in URL") in input.go instead of
// here, so this file never needs to change between Actors.
var errEmptyInput = errors.New("empty input")

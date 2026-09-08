package rlenv

import "errors"

var (
	errArrivalThreshold        = errors.New("rlenv: ArrivalThreshold must be > 0")
	errStepTimeout             = errors.New("rlenv: StepTimeout must be > 0")
	errNilAgent                = errors.New("rlenv: agent must not be nil")
	errNilRegistry             = errors.New("rlenv: registry must not be nil")
	errPositionUnknown         = errors.New("rlenv: agent position not yet initialized")
	errSeederRequiresSeedAgent = errors.New("rlenv: Config.Seeder is set but agent does not implement SeedAgent")
)

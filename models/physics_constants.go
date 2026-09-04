package models

// Player dimension constants.
const (
	PlayerWidth              = 0.6
	PlayerHeight             = 1.8
	PlayerEyeHeight          = 1.62
	PlayerEyeHeightCrouching = 1.27
	PlayerEyeHeightSneaking  = 1.27
	PlayerHeightSneaking     = 1.5
)

// HappyGhastWidth and HappyGhastHeight are the adult happy ghast hitbox
// dimensions (1.21.6+), confirmed against the decompiled source and
// mc-data-gen's extracted entity JSON. Duplicated from physics.HappyGhastWidth/
// Height since models can't import physics (physics imports models).
const (
	HappyGhastWidth  = 4.0
	HappyGhastHeight = 4.0
)

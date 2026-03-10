// Package embeds provides embedded filesystem assets bundled into the binary.
package embeds

import "embed"

// Skills contains the sample skills shipped with picobot by default.
// Each skill is a directory with a skill.md file.
//
//go:embed skills/*
var Skills embed.FS

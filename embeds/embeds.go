// Package embeds provides embedded filesystem assets bundled into the binary.
package embeds

import "embed"

// Skills contains the sample skills shipped with picobot by default.
// Each skill is a directory with a SKILL.md file.
//
//go:embed skills/*
var Skills embed.FS

// UI contains the web UI frontend HTML/CSS/JS files.
//
//go:embed ui/*
var UI embed.FS

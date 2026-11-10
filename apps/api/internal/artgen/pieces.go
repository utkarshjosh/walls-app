package artgen

// Style names double as category slugs, except mesh, which is filed under
// "gradients".
const (
	StyleMesh     = "mesh"
	StyleHorizon  = "horizon"
	StyleDunes    = "dunes"
	StyleContours = "contours"
	StyleAurora   = "aurora"
)

// Category returns the catalog category (slug and display name) for a style.
func Category(style string) (slug, name string) {
	switch style {
	case StyleMesh:
		return "gradients", "Gradients"
	case StyleHorizon:
		return "horizons", "Horizons"
	case StyleDunes:
		return "dunes", "Dunes"
	case StyleContours:
		return "contours", "Contours"
	case StyleAurora:
		return "aurora", "Aurora"
	}
	return style, style
}

// Catalog returns the starter set in feed order. The order is hand-arranged
// so that neighbouring items differ in style and in tone (light/dark,
// warm/cool) rather than clumping. Seeds are fixed: changing one changes that
// picture, and therefore its id.
func Catalog() []Piece {
	return []Piece{
		{
			Slug: "low-tide", Title: "Low Tide", Style: StyleMesh, Seed: 11,
			Tags: []string{"warm", "calm", "coastal"},
			build: mesh(meshSpec{
				stops: []string{"#1f6f7a", "#4fa3a0", "#a8d5c2", "#f3e7cf", "#f6c7a0", "#ec9274"},
				angle: 28, stretch: 0.85, warp: 0.30, freq: 0.8, swirl: 0.6, depth: 0.03, sheen: 0.03,
			}),
		},
		{
			Slug: "far-ridge", Title: "Far Ridge", Style: StyleHorizon, Seed: 21,
			Tags: []string{"dusk", "mountains", "warm"},
			build: horizon(horizonSpec{
				sky: []string{"#1d2b4f", "#4b4a80", "#b56d86", "#f2a36c"},
				sun: "#ffd7a0", disc: "#fff3da", sunX: 0.66, sunY: 0.565, sunR: 0.026, glow: 0.62,
				far: "#9a7f9d", near: "#171a30", layers: 4, horizon: 0.62, rough: 1, mist: 0.75,
			}),
		},
		{
			Slug: "north-lights", Title: "North Lights", Style: StyleAurora, Seed: 31, Dark: true,
			Tags: []string{"night", "green", "stars"},
			build: aurora(auroraSpec{
				sky:     []string{"#02040c", "#04101c", "#0a2230"},
				curtain: []string{"#3df2a4", "#22b9c6", "#5a4fd2"},
				ground:  "#02060a", curtains: 3, power: 0.85, sweep: 0.6,
			}),
		},
		{
			Slug: "quiet-sands", Title: "Quiet Sands", Style: StyleDunes, Seed: 41,
			Tags: []string{"sand", "warm", "light"},
			build: dunes(dunesSpec{
				sky:    []string{"#e9d3bd", "#f3e2cf", "#f8ecdc"},
				lit:    []string{"#f3d8b4", "#eec190", "#e3a26b", "#d0814f", "#b0623c"},
				shade:  []string{"#dcb894", "#c9966e", "#ac7252", "#87523f", "#5f372e"},
				layers: 7, top: 0.46, light: -1, glow: "#fff4e0", swell: 1,
			}),
		},
		{
			Slug: "night-survey", Title: "Night Survey", Style: StyleContours, Seed: 51, Dark: true,
			Tags: []string{"dark", "lines", "minimal"},
			build: contours(contoursSpec{
				ground: "#0d1015", ground2: "#141a22", line: "#5f6c7a", accent: "#ff8a4c",
				scale: 1.25, step: 0.05, peak: 0.17, weight: 1,
			}),
		},
		{
			Slug: "slow-dawn", Title: "Slow Dawn", Style: StyleMesh, Seed: 61, Wide: true,
			Tags: []string{"sunrise", "warm", "wide"},
			build: mesh(meshSpec{
				stops: []string{"#141a44", "#2b2f7a", "#6a3f98", "#c2568f", "#f0806c", "#f9bd7c", "#fbe6bf"},
				angle: 15, stretch: 0.9, warp: 0.30, freq: 0.7, swirl: 0.7, depth: 0.03, sheen: 0.03,
			}),
		},
		{
			Slug: "pale-hour", Title: "Pale Hour", Style: StyleHorizon, Seed: 71,
			Tags: []string{"mist", "morning", "light"},
			build: horizon(horizonSpec{
				sky: []string{"#bccbd9", "#d4dde4", "#ebe7e1", "#f6eee2"},
				sun: "#fff8ea", disc: "#fffdf6", sunX: 0.30, sunY: 0.40, sunR: 0.030, glow: 0.5,
				far: "#c9d3da", near: "#566a80", layers: 4, horizon: 0.64, rough: 0.9, mist: 0.95,
			}),
		},
		{
			Slug: "deep-current", Title: "Deep Current", Style: StyleMesh, Seed: 81, Dark: true,
			Tags: []string{"dark", "blue", "deep"},
			build: mesh(meshSpec{
				stops: []string{"#040915", "#0a1f4d", "#124a9c", "#1f9bc4", "#3fd6c8", "#1a6fb0", "#1c2a7a", "#070c22"},
				angle: 60, stretch: 0.75, warp: 0.38, freq: 0.8, swirl: 0.9, depth: 0.03, sheen: 0.03,
			}),
		},
		{
			Slug: "rose-drift", Title: "Rose Drift", Style: StyleDunes, Seed: 91,
			Tags: []string{"pink", "soft", "flowing"},
			build: dunes(dunesSpec{
				sky:    []string{"#f3cfc8", "#f8e0d6", "#fbece2"},
				lit:    []string{"#f6cdc2", "#eeaea8", "#de8a92", "#c26b82", "#9a4f6e"},
				shade:  []string{"#dba9a8", "#c4858f", "#a5667c", "#7f4a67", "#5a324f"},
				layers: 7, top: 0.44, light: 1, glow: "#fff1ea", swell: 1.1,
			}),
		},
		{
			Slug: "paper-trail", Title: "Paper Trail", Style: StyleContours, Seed: 101,
			Tags: []string{"paper", "light", "minimal"},
			build: contours(contoursSpec{
				ground: "#efe8dc", ground2: "#e7ddcd", line: "#a39582", accent: "#c8553d",
				scale: 1.0, step: 0.06, peak: 0.17, weight: 1,
			}),
		},
		{
			Slug: "violet-veil", Title: "Violet Veil", Style: StyleAurora, Seed: 111, Dark: true,
			Tags: []string{"night", "violet", "stars"},
			build: aurora(auroraSpec{
				sky:     []string{"#04030d", "#0b0920", "#17123a"},
				curtain: []string{"#d27bff", "#8a63ff", "#3f8fff"},
				ground:  "#04030a", curtains: 3, power: 0.8, sweep: -0.5,
			}),
		},
		{
			Slug: "long-dusk", Title: "Long Dusk", Style: StyleHorizon, Seed: 121, Wide: true,
			Tags: []string{"dusk", "teal", "wide"},
			build: horizon(horizonSpec{
				sky: []string{"#0f2440", "#1f4e66", "#5f8f8a", "#d9b077", "#f6d6a0"},
				sun: "#ffd08a", disc: "#fff0cf", sunX: 0.27, sunY: 0.585, sunR: 0.024, glow: 0.6,
				far: "#6f9a96", near: "#0b1a26", layers: 5, horizon: 0.63, rough: 1, mist: 0.8,
			}),
		},
		{
			Slug: "glass-morning", Title: "Glass Morning", Style: StyleMesh, Seed: 131,
			Tags: []string{"light", "cool", "soft"},
			build: mesh(meshSpec{
				stops: []string{"#8fb6ee", "#b5c4f4", "#d7cdf6", "#f4d3e6", "#fbe3da", "#fdf4e8"},
				angle: 35, stretch: 0.9, warp: 0.40, freq: 0.8, swirl: 1.0, depth: 0.02, sheen: 0.025,
			}),
		},
		{
			Slug: "blue-hour", Title: "Blue Hour", Style: StyleDunes, Seed: 141, Dark: true,
			Tags: []string{"dusk", "blue", "dark"},
			build: dunes(dunesSpec{
				sky:    []string{"#141b36", "#2b3560", "#5c5a88", "#8f7a9c"},
				lit:    []string{"#8480ac", "#666a9e", "#4c548a", "#363f73", "#262d59"},
				shade:  []string{"#5a5a88", "#444a78", "#313862", "#20274b", "#131834"},
				layers: 7, top: 0.47, light: 1, glow: "#c9a6b8", swell: 1,
			}),
		},
		{
			Slug: "last-light", Title: "Last Light", Style: StyleHorizon, Seed: 151, Dark: true,
			Tags: []string{"sunset", "red", "dark"},
			build: horizon(horizonSpec{
				sky: []string{"#0f0a1f", "#3a1240", "#8f1f4a", "#e8552d", "#ffb347"},
				sun: "#ffb65c", disc: "#ffe2a8", sunX: 0.48, sunY: 0.655, sunR: 0.040, glow: 0.7,
				far: "#6a2550", near: "#0a0612", layers: 4, horizon: 0.66, rough: 1.1, mist: 0.45,
			}),
		},
		{
			Slug: "deep-chart", Title: "Deep Chart", Style: StyleContours, Seed: 161, Dark: true,
			Tags: []string{"navy", "lines", "calm"},
			build: contours(contoursSpec{
				ground: "#0a1628", ground2: "#0f2238", line: "#3f6787", accent: "#4fd1c5",
				scale: 1.0, step: 0.045, peak: 0.17, weight: 1,
			}),
		},
		{
			Slug: "ember-field", Title: "Ember Field", Style: StyleMesh, Seed: 171, Dark: true,
			Tags: []string{"dark", "warm", "glow"},
			build: mesh(meshSpec{
				stops: []string{"#0d0609", "#2a0c18", "#6e1220", "#c43a1c", "#f28a2e", "#c9491d", "#4a0f1c", "#10070b"},
				angle: 110, stretch: 1.05, warp: 0.36, freq: 0.8, swirl: 0.9, depth: 0.03, sheen: 0.03,
			}),
		},
		{
			Slug: "open-desert", Title: "Open Desert", Style: StyleDunes, Seed: 181, Wide: true,
			Tags: []string{"desert", "golden", "wide"},
			build: dunes(dunesSpec{
				sky:    []string{"#f0b877", "#f6cf9c", "#fae3c1"},
				lit:    []string{"#f8d29a", "#f2b36a", "#e6924a", "#d17335", "#b05629"},
				shade:  []string{"#dba878", "#c4875a", "#a56445", "#804636", "#5a2e2c"},
				layers: 8, top: 0.45, light: -1, glow: "#fff0cc", swell: 1.05,
			}),
		},
		{
			Slug: "polar-glow", Title: "Polar Glow", Style: StyleAurora, Seed: 191, Dark: true,
			Tags: []string{"night", "teal", "calm"},
			build: aurora(auroraSpec{
				sky:     []string{"#020812", "#051522", "#0a2836"},
				curtain: []string{"#62f0df", "#2f9cf0", "#3b50d0"},
				ground:  "#02070c", curtains: 2, power: 1.1, sweep: 0.8,
			}),
		},
		{
			Slug: "salt-haze", Title: "Salt Haze", Style: StyleHorizon, Seed: 201,
			Tags: []string{"desert", "light", "warm"},
			build: horizon(horizonSpec{
				sky: []string{"#8fbfca", "#bcd9d2", "#e6e2c6", "#f4e2bf"},
				sun: "#fff3d6", sunX: 0.22, sunY: 0.50, glow: 0.55,
				far: "#dcc7a4", near: "#96563a", layers: 4, horizon: 0.63, rough: 0.8, mist: 0.85,
			}),
		},
		{
			Slug: "moss-light", Title: "Moss Light", Style: StyleMesh, Seed: 211,
			Tags: []string{"green", "natural", "calm"},
			build: mesh(meshSpec{
				stops: []string{"#1f4034", "#3f7052", "#86ad78", "#cbd89a", "#f0e9b8", "#f7f1dc"},
				angle: 140, stretch: 0.8, warp: 0.42, freq: 0.9, swirl: 1.1, depth: 0.03, sheen: 0.03,
			}),
		},
		{
			Slug: "sage-hills", Title: "Sage Hills", Style: StyleDunes, Seed: 221,
			Tags: []string{"green", "hills", "calm"},
			build: dunes(dunesSpec{
				sky:    []string{"#d5e2cf", "#e4ecdb", "#f0f3e6"},
				lit:    []string{"#cfdcb8", "#adc698", "#86aa7d", "#628b68", "#446c55"},
				shade:  []string{"#b0c2a0", "#8ca685", "#68896c", "#476a56", "#2d4b42"},
				layers: 7, top: 0.45, light: -1, glow: "#fbfbe8", swell: 1.15,
			}),
		},
		{
			Slug: "sky-river", Title: "Sky River", Style: StyleAurora, Seed: 231, Dark: true, Wide: true,
			Tags: []string{"night", "stars", "wide"},
			build: aurora(auroraSpec{
				sky:     []string{"#02050d", "#04121f", "#0a2433"},
				curtain: []string{"#4df0b0", "#2fc0d8", "#6a5ae0"},
				ground:  "#02060a", curtains: 4, power: 0.8, sweep: 0.45,
			}),
		},
		{
			Slug: "fern-line", Title: "Fern Line", Style: StyleContours, Seed: 241, Dark: true,
			Tags: []string{"green", "dark", "lines"},
			build: contours(contoursSpec{
				ground: "#0e1913", ground2: "#16271d", line: "#466a56", accent: "#d9c36a",
				scale: 1.4, step: 0.055, peak: 0.17, weight: 1,
			}),
		},
		{
			Slug: "rose-quartz", Title: "Rose Quartz", Style: StyleMesh, Seed: 251,
			Tags: []string{"pink", "soft", "warm"},
			build: mesh(meshSpec{
				stops: []string{"#8f7fc4", "#c58bbd", "#ee9aa9", "#f7bdb0", "#fbdcc6", "#fdf0e2"},
				angle: 200, stretch: 0.9, warp: 0.40, freq: 0.8, swirl: 1.0, depth: 0.025, sheen: 0.03,
			}),
		},
		{
			Slug: "moonrise", Title: "Moonrise", Style: StyleHorizon, Seed: 261, Dark: true,
			Tags: []string{"night", "moon", "stars"},
			build: horizon(horizonSpec{
				sky: []string{"#040816", "#0a1630", "#16294a", "#243f63"},
				sun: "#9db8e0", disc: "#f4f1e6", sunX: 0.72, sunY: 0.30, sunR: 0.034, glow: 0.38,
				far: "#27405f", near: "#04070f", layers: 4, horizon: 0.66, rough: 1, mist: 0.6, stars: true,
			}),
		},
		{
			Slug: "black-sand", Title: "Black Sand", Style: StyleDunes, Seed: 271, Dark: true,
			Tags: []string{"dark", "minimal", "mono"},
			build: dunes(dunesSpec{
				sky:    []string{"#0b0b0e", "#141316", "#211e20"},
				lit:    []string{"#5c5049", "#52463f", "#473c37", "#3c3330", "#322b29"},
				shade:  []string{"#1f1c1d", "#1a1718", "#151314", "#100e10", "#0a090a"},
				layers: 7, top: 0.44, light: -1, glow: "#4a3a30", swell: 1.1,
			}),
		},
		{
			Slug: "night-bloom", Title: "Night Bloom", Style: StyleMesh, Seed: 281, Dark: true, Wide: true,
			Tags: []string{"dark", "vivid", "wide"},
			build: mesh(meshSpec{
				stops: []string{"#05040d", "#0e1648", "#2c36c0", "#6a24a8", "#c22a90", "#f2607e", "#7a1f78", "#120a2e", "#05040d"},
				angle: 8, stretch: 0.8, warp: 0.34, freq: 0.55, swirl: 0.9, depth: 0.03, sheen: 0.03,
			}),
		},
		{
			Slug: "solar-wind", Title: "Solar Wind", Style: StyleAurora, Seed: 291, Dark: true,
			Tags: []string{"night", "vivid", "stars"},
			build: aurora(auroraSpec{
				sky:     []string{"#04040a", "#0a0d1c", "#141a2c"},
				curtain: []string{"#8dffb8", "#ff7fb0", "#b04fd8"},
				ground:  "#030408", curtains: 3, power: 0.8, sweep: -0.7,
			}),
		},
		{
			Slug: "wide-survey", Title: "Wide Survey", Style: StyleContours, Seed: 301, Wide: true,
			Tags: []string{"light", "lines", "wide"},
			build: contours(contoursSpec{
				ground: "#e9ebee", ground2: "#dde1e6", line: "#8f9aa8", accent: "#2f6fde",
				scale: 1.1, step: 0.05, peak: 0.17, weight: 1,
			}),
		},
	}
}

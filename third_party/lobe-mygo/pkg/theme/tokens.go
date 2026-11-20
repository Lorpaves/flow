package theme

//go:generate bun ../../scripts/gen-tokens.ts

func Light() *Tokens {
	t := lightTokens
	return &t
}

func Dark() *Tokens {
	t := darkTokens
	return &t
}

// Customize replaces the palette every component reads, Light's with what
// light returns and Dark's with what dark returns, each given Lobe's own. An
// application calls it once, before it builds a frame (Flow patch).
func Customize(light, dark func(Tokens) Tokens) {
	lightTokens, darkTokens = light(lightTokens), dark(darkTokens)
	lightTheme, darkTheme = lightTokens.Theme(), darkTokens.Theme()
}

func ColorTokenNames() []string {
	return append([]string(nil), colorTokenNames...)
}

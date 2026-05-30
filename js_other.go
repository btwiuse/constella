//go:build !js || !wasm

package constella

func maybeRegisterJS(c *Constella) {
	// no-op on non-JS platforms
}

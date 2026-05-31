//go:build js && wasm

package constella

import (
	"encoding/json"
	"syscall/js"
)

func maybeRegisterJS(c *Constella) {
	c.Counter.OnUpdate = func() {
		snap := c.Counter.Snapshot()
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return
		}
		js.Global().Get("window").Call("dispatchEvent",
			js.Global().Get("CustomEvent").New("counter_update", map[string]any{
				"detail": string(data),
			}))
	}

	addFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		c.Counter.Increment()
		return nil
	})
	getFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		snap := c.Counter.Snapshot()
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return nil
		}
		return string(data)
	})

	counterObj := js.Global().Get("Object").New()
	counterObj.Set("add", addFn)
	counterObj.Set("get", getFn)
	js.Global().Set("counter", counterObj)

	// Prevent GC of the js.Func wrappers
	js.Global().Get("window").Set("__counter_add", addFn)
	js.Global().Get("window").Set("__counter_get", getFn)

	js.Global().Get("window").Call("dispatchEvent",
		js.Global().Get("CustomEvent").New("counter_ready"))
}

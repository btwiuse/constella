//go:build js && wasm

package constella

import (
	"encoding/json"
	"syscall/js"
)

var global = js.Global()

func maybeRegisterJS(c *Constella) {
	c.Counter.OnUpdate = func() {
		snap := c.Counter.Snapshot()
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return
		}
		event := global.Get("CustomEvent").New("counter_update", map[string]any{
			"detail": string(data),
		})
		global.Call("dispatchEvent", event)
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

	counterObj := global.Get("Object").New()
	counterObj.Set("add", addFn)
	counterObj.Set("get", getFn)
	global.Set("counter", counterObj)

	// Prevent GC of the js.Func wrappers
	global.Set("__counter_add", addFn)
	global.Set("__counter_get", getFn)

	event := global.Get("CustomEvent").New("counter_ready")
	global.Call("dispatchEvent", event)
}

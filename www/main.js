if (!WebAssembly.instantiateStreaming) {
  // polyfill
  WebAssembly.instantiateStreaming = async (resp, importObject) => {
    const source = await (await resp).arrayBuffer();
    return await WebAssembly.instantiate(source, importObject);
  };
}

function extractGoEnv(search = window.location.search) {
  const params = new URLSearchParams(search);
  const env = {};
  for (const [key, value] of params.entries()) {
    if (!key.startsWith("env.")) continue;
    const envKey = key.slice(4);
    if (!/^[A-Z0-9_]+$/i.test(envKey)) continue;
    env[envKey] = value;
  }
  return env;
}

const params = new URLSearchParams(location.search);
const bootnode = params.get("bootnode") ||
  "/dns/constella-production.up.railway.app/tcp/443/wss";

const go = new Go();
go.env = {
  RELAY: "https://pub.webtransport.fun",
  ...extractGoEnv(),
};
go.argv = ["constella.wasm", bootnode];
let mod, inst;
WebAssembly.instantiateStreaming(fetch("./constella.wasm"), go.importObject)
  .then((result) => {
    mod = result.module;
    inst = result.instance;
    run();
  })
  .catch((err) => {
    console.error(err);
  });

async function run() {
  console.clear();
  await go.run(inst);
  inst = await WebAssembly.instantiate(mod, go.importObject); // reset instance
}

window.addEventListener("counter_ready", () => {
  document.getElementById("runButton").disabled = false;
  document.getElementById("runButton").addEventListener("click", () => {
    counter.add();
  });
});

window.addEventListener("counter_update", (e) => {
  const data = JSON.parse(e.detail);
  document.getElementById("counterSum").textContent = data.sum;
  document.title = data.sum;
  console.log(e.detail);
});

build:
	go mod tidy
	CGO_ENABLED=0 go build -o /tmp/ ./cmd/constella

build-wasm:
	go mod tidy
	cp $$(go env GOROOT)/lib/wasm/wasm_exec.js ./www
	GOOS=js GOARCH=wasm go build -o www/constella.wasm ./cmd/constella

run:
	# The constella libp2p address could be different from the HTTP listening address
	# and will not be displayed in the terminal, but you can find it
	# in the http response to the /info endpoint
	/tmp/constella

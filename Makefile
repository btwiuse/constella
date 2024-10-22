build:
	go build -o /tmp/ ./cmd/constella

run:
	# The constella libp2p address could be different from the HTTP listening address
	# and will not be displayed in the terminal, but you can find it
	# in the http response to the /info endpoint
	/tmp/constella

bind:
	@go run . -pkg main -o generated/$(name).go testdata/src/$(name).h

test:
	@go test .
